FROM python:3.14.7-slim

WORKDIR /tests
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY test.py ./
CMD ["python", "test.py"]
