import json
import os
from urllib.error import HTTPError
from urllib.request import Request, urlopen


def _endpoint_environment():
    return {key: value for key, value in os.environ.items() if key == "AWS_ENDPOINT_URL" or key.startswith("AWS_ENDPOINT_URL_")}


def _restore(snapshot):
    for key in list(_endpoint_environment()):
        del os.environ[key]
    os.environ.update(snapshot)


def _request(url, method, payload=None):
    data = None if payload is None else json.dumps(payload).encode()
    request = Request(url, data=data, method=method)
    if data is not None:
        request.add_header("Content-Type", "application/json")
    try:
        with urlopen(request) as response:
            body = response.read()
    except HTTPError as error:
        raise RuntimeError(f"{method} {url} failed: {error.code} {error.read().decode()}") from error
    return json.loads(body) if body else None


class FixtureSession:
    @classmethod
    def start(cls, *, server_url):
        created = _request(f"{server_url}/__fixture/sessions", "POST")
        if not created or not created.get("sessionId") or not created.get("endpoint"):
            raise RuntimeError("invalid session response")
        saved = _endpoint_environment()
        for key in saved:
            del os.environ[key]
        os.environ["AWS_ENDPOINT_URL"] = created["endpoint"]
        return cls(server_url, created["sessionId"], saved)

    def __init__(self, server_url, session_id, saved_endpoints):
        self.server_url = server_url
        self.session_id = session_id
        self.saved_endpoints = saved_endpoints

    def _control(self, suffix):
        return f"{self.server_url}/__fixture/sessions/{self.session_id}{suffix}"

    def load_scenario(self, path):
        return _request(self._control("/scenario"), "POST", {"path": path})

    def reset(self):
        return _request(self._control("/reset"), "POST")

    def requests(self):
        return _request(self._control("/requests"), "GET")

    def destroy(self):
        try:
            _request(self._control(""), "DELETE")
        finally:
            _restore(self.saved_endpoints)
