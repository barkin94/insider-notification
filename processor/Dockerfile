FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git

COPY processor/go.mod processor/go.sum processor/
RUN GOPRIVATE=github.com/barkin/insider-notification \
    GONOSUMDB=github.com/barkin/insider-notification \
    go mod download -C processor

COPY processor/ processor/

RUN CGO_ENABLED=0 GOOS=linux go build -C processor -trimpath -o /processor-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /processor-server /processor-server
EXPOSE 8081
ENTRYPOINT ["/processor-server"]
