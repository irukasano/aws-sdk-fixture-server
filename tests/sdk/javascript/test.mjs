import assert from "node:assert/strict";
import test from "node:test";
import { GetSecretValueCommand, SecretsManagerClient } from "@aws-sdk/client-secrets-manager";

import { FixtureSession } from "./fixture-session/index.mjs";

test("AWS SDK client created by the application uses the FixtureSession endpoint", async () => {
  const serverUrl = process.env.AWS_ENDPOINT_URL;
  if (!serverUrl) throw new Error("AWS_ENDPOINT_URL must contain the fixture server URL before FixtureSession.start");

  const health = await fetch(`${serverUrl}/__fixture/health`);
  assert.equal(health.status, 200);

  const fixture = await FixtureSession.start({ serverUrl });
  try {
    await fixture.loadScenario("/scenarios/sdk-core.yml");

    // The application does not receive the helper or an endpoint override.
    // Its normally constructed SDK client resolves AWS_ENDPOINT_URL itself.
    const client = new SecretsManagerClient({ region: process.env.AWS_REGION });
    const result = await client.send(new GetSecretValueCommand({ SecretId: "test/db" }));
    assert.equal(result.Name, "test/db");
    assert.equal(result.SecretString, '{"host":"db"}');
  } finally {
    await fixture.destroy();
  }
});
