import assert from "node:assert/strict";
import { createServer } from "node:http";
import test from "node:test";

import { FixtureSession } from "./index.mjs";

const endpointKeys = () => Object.keys(process.env).filter(
  (key) => key === "AWS_ENDPOINT_URL" || key.startsWith("AWS_ENDPOINT_URL_"),
);

function snapshotEndpointEnvironment() {
  return Object.fromEntries(endpointKeys().map((key) => [key, process.env[key]]));
}

function restoreEndpointEnvironment(snapshot) {
  for (const key of endpointKeys()) delete process.env[key];
  Object.assign(process.env, snapshot);
}

async function withFixtureServer(handler, run) {
  const server = createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  const serverUrl = `http://127.0.0.1:${address.port}`;
  try {
    await run(serverUrl);
  } finally {
    await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  }
}

function collectBody(request) {
  return new Promise((resolve, reject) => {
    let body = "";
    request.setEncoding("utf8");
    request.on("data", (chunk) => { body += chunk; });
    request.on("end", () => resolve(body));
    request.on("error", reject);
  });
}

test("FixtureSession controls a session and restores every endpoint variable", async () => {
  const original = snapshotEndpointEnvironment();
  const previousRegion = process.env.AWS_REGION;
  process.env.AWS_ENDPOINT_URL = "https://previous.example";
  process.env.AWS_ENDPOINT_URL_S3 = "https://s3.previous.example";
  process.env.AWS_ENDPOINT_URL_FUTURE_SERVICE = "https://future.previous.example";
  process.env.AWS_REGION = "us-test-1";

  const calls = [];
  try {
    await withFixtureServer(async (request, response) => {
      const body = await collectBody(request);
      calls.push({ method: request.method, path: request.url, body });
      if (request.method === "POST" && request.url === "/__fixture/sessions") {
        response.writeHead(201, { "content-type": "application/json" });
        response.end(JSON.stringify({
          sessionId: "session-1",
          endpoint: `http://${request.headers.host}/__fixture/sessions/session-1/aws`,
          issuer: `http://${request.headers.host}/__fixture/sessions/session-1/oidc/ap-northeast-1_test`,
        }));
        return;
      }
      if (request.method === "POST" && request.url === "/__fixture/sessions/session-1/scenario") {
        response.writeHead(200, { "content-type": "application/json" });
        response.end(JSON.stringify({ scenario: "happy", loaded: true, issuer: `http://${request.headers.host}/__fixture/sessions/session-1/oidc/ap-northeast-1_test` }));
        return;
      }
      if (request.method === "POST" && request.url === "/__fixture/sessions/session-1/reset") {
        response.writeHead(200).end();
        return;
      }
      if (request.method === "GET" && request.url === "/__fixture/sessions/session-1/requests") {
        response.writeHead(200, { "content-type": "application/json" });
        response.end(JSON.stringify([{ service: "s3", operation: "PutObject" }]));
        return;
      }
      if (request.method === "DELETE" && request.url === "/__fixture/sessions/session-1") {
        response.writeHead(204).end();
        return;
      }
      response.writeHead(404).end();
    }, async (serverUrl) => {
      const fixture = await FixtureSession.start({ serverUrl });
      assert.equal(fixture.sessionId, "session-1");
      assert.equal(fixture.issuer, `${serverUrl}/__fixture/sessions/session-1/oidc/ap-northeast-1_test`);
      assert.equal(process.env.AWS_ENDPOINT_URL, `${serverUrl}/__fixture/sessions/session-1/aws`);
      assert.equal(process.env.AWS_ENDPOINT_URL_S3, undefined);
      assert.equal(process.env.AWS_ENDPOINT_URL_FUTURE_SERVICE, undefined);
      assert.equal(process.env.AWS_REGION, "us-test-1");

      await fixture.loadScenario("/scenarios/happy.yml");
      await fixture.reset();
      assert.deepEqual(await fixture.requests(), [{ service: "s3", operation: "PutObject" }]);
      await fixture.destroy();
    });

    assert.deepEqual(calls, [
      { method: "POST", path: "/__fixture/sessions", body: "" },
      { method: "POST", path: "/__fixture/sessions/session-1/scenario", body: JSON.stringify({ path: "/scenarios/happy.yml" }) },
      { method: "POST", path: "/__fixture/sessions/session-1/reset", body: "" },
      { method: "GET", path: "/__fixture/sessions/session-1/requests", body: "" },
      { method: "DELETE", path: "/__fixture/sessions/session-1", body: "" },
    ]);
    assert.equal(process.env.AWS_ENDPOINT_URL, "https://previous.example");
    assert.equal(process.env.AWS_ENDPOINT_URL_S3, "https://s3.previous.example");
    assert.equal(process.env.AWS_ENDPOINT_URL_FUTURE_SERVICE, "https://future.previous.example");
  } finally {
    restoreEndpointEnvironment(original);
    if (previousRegion === undefined) delete process.env.AWS_REGION;
    else process.env.AWS_REGION = previousRegion;
  }
});

test("FixtureSession.start leaves endpoint variables untouched when creation fails", async () => {
  const original = snapshotEndpointEnvironment();
  process.env.AWS_ENDPOINT_URL = "https://previous.example";
  process.env.AWS_ENDPOINT_URL_SECRETS_MANAGER = "https://secrets.previous.example";
  const before = snapshotEndpointEnvironment();
  try {
    await withFixtureServer((_, response) => response.writeHead(500).end("no session"), async (serverUrl) => {
      await assert.rejects(FixtureSession.start({ serverUrl }));
    });
    assert.deepEqual(snapshotEndpointEnvironment(), before);
  } finally {
    restoreEndpointEnvironment(original);
  }
});

test("FixtureSession.destroy restores endpoint variables even when deletion fails", async () => {
  const original = snapshotEndpointEnvironment();
  process.env.AWS_ENDPOINT_URL = "https://previous.example";
  process.env.AWS_ENDPOINT_URL_SQS = "https://sqs.previous.example";
  const before = snapshotEndpointEnvironment();
  try {
    await withFixtureServer((request, response) => {
      if (request.method === "POST") {
        response.writeHead(201, { "content-type": "application/json" });
        response.end(JSON.stringify({
          sessionId: "session-2",
          endpoint: `http://${request.headers.host}/__fixture/sessions/session-2/aws`,
          issuer: `http://${request.headers.host}/__fixture/sessions/session-2/oidc/ap-northeast-1_test`,
        }));
        return;
      }
      response.writeHead(500).end("delete failed");
    }, async (serverUrl) => {
      const fixture = await FixtureSession.start({ serverUrl });
      await assert.rejects(fixture.destroy());
    });
    assert.deepEqual(snapshotEndpointEnvironment(), before);
  } finally {
    restoreEndpointEnvironment(original);
  }
});
