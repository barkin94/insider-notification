FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY api/go.mod api/go.sum api/
COPY shared/go.mod shared/go.sum shared/
RUN cd api && go mod download

RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

COPY api/ api/
COPY shared/ shared/

RUN cd api && swag init -g cmd/main.go -o docs --parseDependency --parseInternal
RUN cd api && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /api-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /api-server /api-server
EXPOSE 8080
ENTRYPOINT ["/api-server"]
