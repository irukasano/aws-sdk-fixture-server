FROM golang:1.27.1 AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -o /aws-fixture . && CGO_ENABLED=0 go build -o /aws-fixture-healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12
COPY --from=build /aws-fixture /aws-fixture
COPY --from=build /aws-fixture-healthcheck /aws-fixture-healthcheck
COPY defaults /defaults
EXPOSE 4566
ENTRYPOINT ["/aws-fixture"]
