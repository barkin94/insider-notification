FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY processor/go.mod processor/go.sum processor/
COPY shared/go.mod shared/go.sum shared/
RUN cd processor && go mod download

COPY processor/ processor/
COPY shared/ shared/

RUN cd processor && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /processor-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /processor-server /processor-server
EXPOSE 8081
ENTRYPOINT ["/processor-server"]
