import assert from "node:assert/strict";
import test from "node:test";
import { GetSecretValueCommand, SecretsManagerClient } from "@aws-sdk/client-secrets-manager";
import { AdminCreateUserCommand, AdminGetUserCommand, AdminUpdateUserAttributesCommand, CognitoIdentityProviderClient, InitiateAuthCommand, ListUsersCommand, RespondToAuthChallengeCommand } from "@aws-sdk/client-cognito-identity-provider";
import { SendEmailCommand, SESv2Client } from "@aws-sdk/client-sesv2";

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

    const sesv2 = new SESv2Client({ region: process.env.AWS_REGION });
    const email = await sesv2.send(new SendEmailCommand({
      FromEmailAddress: "sender@example.test",
      Destination: { ToAddresses: ["recipient@example.test"] },
      Content: { Simple: { Subject: { Data: "subject" }, Body: { Text: { Data: "body" } } } },
    }));
    assert.equal(email.MessageId, "fixture-message");

    const cognito = new CognitoIdentityProviderClient({ region: process.env.AWS_REGION });
    const commands = [new AdminGetUserCommand({ UserPoolId: "pool", Username: "fixture-user" }), new AdminCreateUserCommand({ UserPoolId: "pool", Username: "fixture-user" }), new AdminUpdateUserAttributesCommand({ UserPoolId: "pool", Username: "fixture-user", UserAttributes: [] }), new ListUsersCommand({ UserPoolId: "pool" }), new InitiateAuthCommand({ ClientId: "client", AuthFlow: "USER_PASSWORD_AUTH", AuthParameters: { USERNAME: "fixture-user", PASSWORD: "fixture-password" } }), new RespondToAuthChallengeCommand({ ClientId: "client", ChallengeName: "PASSWORD_VERIFIER", ChallengeResponses: { USERNAME: "fixture-user", PASSWORD_CLAIM_SIGNATURE: "fixture-signature" } })];
    for (const command of commands) await cognito.send(command);
    const authRequests = await fixture.requests();
    assert.deepEqual(authRequests.find((request) => request.operation === "InitiateAuth")?.parameters.AuthParameters, { USERNAME: "fixture-user", PASSWORD: "fixture-password" });
    assert.deepEqual(authRequests.find((request) => request.operation === "RespondToAuthChallenge")?.parameters.ChallengeResponses, { USERNAME: "fixture-user", PASSWORD_CLAIM_SIGNATURE: "fixture-signature" });
    assert.deepEqual(authRequests.find((request) => request.service === "sesv2" && request.operation === "SendEmail")?.parameters.Content, { Simple: { Subject: { Data: "subject" }, Body: { Text: { Data: "body" } } } });
    await fixture.loadScenario("/scenarios/cognito-errors.yml");
    for (const command of commands) await assert.rejects(cognito.send(command), /fixture error/);
    await fixture.loadScenario("/scenarios/ses-errors.yml");
    await assert.rejects(
      sesv2.send(new SendEmailCommand({
        FromEmailAddress: "rejected@example.test",
        Content: { Raw: { Data: new TextEncoder().encode("raw message") } },
      })),
      (error) => error.name === "MessageRejected",
    );
  } finally {
    await fixture.destroy();
  }
});
