import os
import urllib.request

import boto3

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

        bedrock = boto3.client("bedrock-runtime")
        invoke_result = bedrock.invoke_model(
            modelId="test-model",
            body=b"{}",
            contentType="application/json",
            accept="application/json",
        )
        assert b"fixture response" in invoke_result["body"].read()

        history = fixture.requests()
        assert any(
            item["service"] == "s3" and item["operation"] == "PutObject"
            for item in history
        )
    finally:
        fixture.destroy()
