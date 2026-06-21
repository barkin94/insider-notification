FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git

COPY api/go.mod api/go.sum api/
RUN GOPRIVATE=github.com/barkin/insider-notification \
    GONOSUMDB=github.com/barkin/insider-notification \
    go mod download -C api

RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

COPY api/ api/

RUN cd api && swag init -g cmd/main.go -o docs --parseDependency --parseInternal
RUN CGO_ENABLED=0 GOOS=linux go build -C api -trimpath -o /api-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /api-server /api-server
EXPOSE 8080
ENTRYPOINT ["/api-server"]
