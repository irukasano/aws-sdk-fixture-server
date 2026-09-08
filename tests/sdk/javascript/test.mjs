import assert from "node:assert/strict";
import { GetSecretValueCommand, SecretsManagerClient } from "@aws-sdk/client-secrets-manager";

const endpoint = process.env.AWS_ENDPOINT_URL;
if (!endpoint) throw new Error("AWS_ENDPOINT_URL must be set");

for (let attempt = 0; attempt < 60; attempt += 1) {
  try {
    const health = await fetch(`${endpoint}/__fixture/health`);
    if (health.ok) break;
  } catch {}
  if (attempt === 59) throw new Error("fixture server did not become healthy");
  await new Promise((resolve) => setTimeout(resolve, 1000));
}

const loaded = await fetch(`${endpoint}/__fixture/scenario`, {
  method: "POST",
  headers: { "content-type": "application/json" },
  body: JSON.stringify({ path: "/scenarios/sdk-core.yml" }),
});
assert.equal(loaded.status, 200, await loaded.text());

const client = new SecretsManagerClient({ endpoint, region: process.env.AWS_REGION });
const result = await client.send(new GetSecretValueCommand({ SecretId: "test/db" }));
assert.equal(result.Name, "test/db");
assert.equal(result.SecretString, '{"host":"db"}');
