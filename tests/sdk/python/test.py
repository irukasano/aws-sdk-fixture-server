import os
import urllib.request

import boto3
import pytest

from fixture_session import FixtureSession


def test_application_sdk_client_uses_fixture_session_endpoint():
    server_url = os.environ["AWS_ENDPOINT_URL"]
    with urllib.request.urlopen(f"{server_url}/__fixture/health") as response:
        assert response.status == 200

    fixture = FixtureSession.start(server_url=server_url)
    try:
        fixture.load_scenario("/scenarios/sdk-core.yml")

        # The application does not receive the helper or endpoint_url. boto3
        # resolves the endpoint configured by FixtureSession for this process.
        s3 = boto3.client("s3")
        get_result = s3.get_object(Bucket="test-bucket", Key="hello.txt")
        assert get_result["Body"].read() == b"hello world"
        s3.put_object(Bucket="test-bucket", Key="upload.txt", Body=b"upload")

        sqs = boto3.client("sqs")
        send_result = sqs.send_message(
            QueueUrl="https://sqs.us-east-1.amazonaws.com/123/test",
            MessageBody="hello",
        )
        assert send_result["MessageId"] == "message-1"

        sesv2 = boto3.client("sesv2")
        email_result = sesv2.send_email(
            FromEmailAddress="sender@example.test",
            Destination={"ToAddresses": ["recipient@example.test"]},
            Content={"Simple": {"Subject": {"Data": "subject"}, "Body": {"Text": {"Data": "body"}}}},
        )
        assert email_result["MessageId"] == "fixture-message"

        bedrock = boto3.client("bedrock-runtime")
        invoke_result = bedrock.invoke_model(
            modelId="test-model",
            body=b"{}",
            contentType="application/json",
            accept="application/json",
        )
        assert b"fixture response" in invoke_result["body"].read()

        cognito = boto3.client("cognito-idp")
        calls = [
            lambda: cognito.admin_get_user(UserPoolId="pool", Username="fixture-user"),
            lambda: cognito.admin_create_user(UserPoolId="pool", Username="fixture-user"),
            lambda: cognito.admin_update_user_attributes(UserPoolId="pool", Username="fixture-user", UserAttributes=[]),
            lambda: cognito.list_users(UserPoolId="pool"),
            lambda: cognito.initiate_auth(ClientId="client", AuthFlow="USER_PASSWORD_AUTH", AuthParameters={"USERNAME": "fixture-user", "PASSWORD": "fixture-password"}),
            lambda: cognito.respond_to_auth_challenge(ClientId="client", ChallengeName="PASSWORD_VERIFIER", ChallengeResponses={"USERNAME": "fixture-user", "PASSWORD_CLAIM_SIGNATURE": "fixture-signature"}),
        ]
        for call in calls:
            call()
        history = fixture.requests()
        assert any(
            item["service"] == "s3" and item["operation"] == "PutObject"
            for item in history
        )
        assert any(
            item["service"] == "sesv2"
            and item["operation"] == "SendEmail"
            and item["parameters"]["Content"]["Simple"]["Subject"]["Data"] == "subject"
            for item in history
        )
        fixture.load_scenario("/scenarios/cognito-errors.yml")
        for call in calls:
            with pytest.raises(cognito.exceptions.NotAuthorizedException):
                call()
        fixture.load_scenario("/scenarios/ses-errors.yml")
        with pytest.raises(sesv2.exceptions.MessageRejected):
            sesv2.send_email(
                FromEmailAddress="rejected@example.test",
                Content={"Raw": {"Data": b"raw message"}},
            )
    finally:
        fixture.destroy()
