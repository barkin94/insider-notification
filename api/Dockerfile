FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /api-server ./api/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /api-server /api-server
EXPOSE 8080
ENTRYPOINT ["/api-server"]
