FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY retryscheduler/go.mod retryscheduler/go.sum retryscheduler/
COPY shared/go.mod shared/go.sum shared/
RUN cd retryscheduler && go mod download

COPY retryscheduler/ retryscheduler/
COPY shared/ shared/

RUN cd retryscheduler && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /retryscheduler-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /retryscheduler-server /retryscheduler-server
ENTRYPOINT ["/retryscheduler-server"]
