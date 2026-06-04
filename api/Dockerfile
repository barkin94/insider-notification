FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6
COPY . .
RUN swag init -g api/cmd/main.go -o api/docs
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /api-server ./api/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /api-server /api-server
EXPOSE 8080
ENTRYPOINT ["/api-server"]
