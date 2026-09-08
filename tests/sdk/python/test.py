import json
import os
import time
import urllib.request

import boto3


endpoint = os.environ["AWS_ENDPOINT_URL"]
for attempt in range(60):
    try:
        with urllib.request.urlopen(f"{endpoint}/__fixture/health") as response:
            if response.status == 200:
                break
    except OSError:
        pass
    if attempt == 59:
        raise RuntimeError("fixture server did not become healthy")
    time.sleep(1)

request = urllib.request.Request(
    f"{endpoint}/__fixture/scenario",
    data=json.dumps({"path": "/scenarios/sdk-core.yml"}).encode(),
    headers={"Content-Type": "application/json"},
    method="POST",
)
with urllib.request.urlopen(request) as response:
    assert response.status == 200

s3 = boto3.client("s3", endpoint_url=endpoint)
get_result = s3.get_object(Bucket="test-bucket", Key="hello.txt")
assert get_result["Body"].read() == b"hello world"
s3.put_object(Bucket="test-bucket", Key="upload.txt", Body=b"upload")

with urllib.request.urlopen(f"{endpoint}/__fixture/requests") as response:
    history = json.load(response)
assert any(
    item["service"] == "s3" and item["operation"] == "PutObject"
    for item in history
)
