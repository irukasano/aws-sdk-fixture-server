.PHONY: test go-test sdk-test

go-test:
	docker run --rm -v aws-fixture-go-cache:/go/pkg/mod -v aws-fixture-go-build-cache:/root/.cache/go-build -v "$(CURDIR):/app" -w /app golang:1.27.1 go test ./...

sdk-test:
	docker compose up --build -d aws-fixture
	docker compose run --rm --no-deps javascript-sdk-tests
	docker compose run --rm --no-deps python-sdk-tests
	docker compose down

test: go-test sdk-test
