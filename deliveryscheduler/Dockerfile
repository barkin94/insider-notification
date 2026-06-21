FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git

COPY deliveryscheduler/go.mod deliveryscheduler/go.sum deliveryscheduler/
RUN GOPRIVATE=github.com/barkin/insider-notification \
    GONOSUMDB=github.com/barkin/insider-notification \
    go mod download -C deliveryscheduler

COPY deliveryscheduler/ deliveryscheduler/

RUN CGO_ENABLED=0 GOOS=linux go build -C deliveryscheduler -trimpath -o /deliveryscheduler-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /deliveryscheduler-server /deliveryscheduler-server
ENTRYPOINT ["/deliveryscheduler-server"]
