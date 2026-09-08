FROM python:3.14.7-slim

WORKDIR /tests
COPY tests/sdk/python/requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY packages/sdk/python/fixture_session /tests/fixture_session
COPY tests/sdk/python/test.py ./
CMD ["pytest", "-q", "test.py"]
