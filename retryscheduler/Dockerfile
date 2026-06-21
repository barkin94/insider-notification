FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git

COPY retryscheduler/go.mod retryscheduler/go.sum retryscheduler/
RUN GOPRIVATE=github.com/barkin/insider-notification \
    GONOSUMDB=github.com/barkin/insider-notification \
    go mod download -C retryscheduler

COPY retryscheduler/ retryscheduler/

RUN CGO_ENABLED=0 GOOS=linux go build -C retryscheduler -trimpath -o /retryscheduler-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /retryscheduler-server /retryscheduler-server
ENTRYPOINT ["/retryscheduler-server"]
