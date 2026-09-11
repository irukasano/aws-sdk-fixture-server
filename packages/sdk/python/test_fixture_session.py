import json
import os
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from fixture_session import FixtureSession


def endpoint_environment():
    return {
        key: value
        for key, value in os.environ.items()
        if key == "AWS_ENDPOINT_URL" or key.startswith("AWS_ENDPOINT_URL_")
    }


def restore_endpoint_environment(snapshot):
    for key in list(endpoint_environment()):
        del os.environ[key]
    os.environ.update(snapshot)


class FixtureServerStub:
    def __init__(self, delete_status=204, create_status=201):
        self.calls = []
        self.delete_status = delete_status
        self.create_status = create_status
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def _respond(self, status, payload=None):
                self.send_response(status)
                if payload is not None:
                    self.send_header("Content-Type", "application/json")
                self.end_headers()
                if payload is not None:
                    self.wfile.write(json.dumps(payload).encode())

            def _record(self):
                content_length = int(self.headers.get("Content-Length", "0"))
                owner.calls.append((self.command, self.path, self.rfile.read(content_length)))

            def do_POST(self):
                self._record()
                if self.path == "/__fixture/sessions":
                    if owner.create_status != 201:
                        self._respond(owner.create_status, {"message": "create failed"})
                        return
                    self._respond(201, {
                        "sessionId": "session-1",
                        "endpoint": f"http://{self.headers['Host']}/__fixture/sessions/session-1/aws",
                    })
                elif self.path == "/__fixture/sessions/session-1/scenario":
                    self._respond(200, {"scenario": "happy", "loaded": True})
                elif self.path == "/__fixture/sessions/session-1/reset":
                    self._respond(200, {})
                else:
                    self._respond(404, {})

            def do_GET(self):
                self._record()
                if self.path == "/__fixture/sessions/session-1/requests":
                    self._respond(200, [{"service": "s3", "operation": "PutObject"}])
                else:
                    self._respond(404, {})

            def do_DELETE(self):
                self._record()
                self._respond(owner.delete_status, None)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)

    @property
    def url(self):
        return f"http://127.0.0.1:{self.server.server_port}"

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_):
        self.server.shutdown()
        self.thread.join()
        self.server.server_close()


class FixtureSessionTests(unittest.TestCase):
    def test_start_controls_session_and_destroy_restores_all_endpoint_variables(self):
        original = endpoint_environment()
        os.environ["AWS_ENDPOINT_URL"] = "https://previous.example"
        os.environ["AWS_ENDPOINT_URL_S3"] = "https://s3.previous.example"
        os.environ["AWS_ENDPOINT_URL_FUTURE_SERVICE"] = "https://future.previous.example"
        before = endpoint_environment()
        try:
            with FixtureServerStub() as server:
                fixture = FixtureSession.start(server_url=server.url)
                self.assertEqual(fixture.session_id, "session-1")
                self.assertEqual(
                    os.environ["AWS_ENDPOINT_URL"],
                    f"{server.url}/__fixture/sessions/session-1/aws",
                )
                self.assertNotIn("AWS_ENDPOINT_URL_S3", os.environ)
                self.assertNotIn("AWS_ENDPOINT_URL_FUTURE_SERVICE", os.environ)
                self.assertEqual(fixture.load_scenario("/scenarios/happy.yml"), {"scenario": "happy", "loaded": True})
                self.assertEqual(fixture.reset(), {})
                self.assertEqual(fixture.requests(), [{"service": "s3", "operation": "PutObject"}])
                self.assertIsNone(fixture.destroy())

                self.assertEqual(
                    server.calls,
                    [
                        ("POST", "/__fixture/sessions", b""),
                        ("POST", "/__fixture/sessions/session-1/scenario", b'{"path": "/scenarios/happy.yml"}'),
                        ("POST", "/__fixture/sessions/session-1/reset", b""),
                        ("GET", "/__fixture/sessions/session-1/requests", b""),
                        ("DELETE", "/__fixture/sessions/session-1", b""),
                    ],
                )
            self.assertEqual(endpoint_environment(), before)
        finally:
            restore_endpoint_environment(original)

    def test_start_failure_leaves_endpoint_environment_unchanged(self):
        original = endpoint_environment()
        os.environ["AWS_ENDPOINT_URL"] = "https://previous.example"
        os.environ["AWS_ENDPOINT_URL_SECRETS_MANAGER"] = "https://secrets.previous.example"
        before = endpoint_environment()
        try:
            with FixtureServerStub(create_status=500) as server:
                with self.assertRaises(Exception):
                    FixtureSession.start(server_url=server.url)
            self.assertEqual(endpoint_environment(), before)
        finally:
            restore_endpoint_environment(original)

    def test_destroy_failure_restores_endpoint_environment_and_raises(self):
        original = endpoint_environment()
        os.environ["AWS_ENDPOINT_URL"] = "https://previous.example"
        os.environ["AWS_ENDPOINT_URL_SQS"] = "https://sqs.previous.example"
        before = endpoint_environment()
        try:
            with FixtureServerStub(delete_status=500) as server:
                fixture = FixtureSession.start(server_url=server.url)
                with self.assertRaises(Exception):
                    fixture.destroy()
            self.assertEqual(endpoint_environment(), before)
        finally:
            restore_endpoint_environment(original)
