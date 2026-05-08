FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /processor-server ./processor/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /processor-server /processor-server
EXPOSE 8081
ENTRYPOINT ["/processor-server"]
