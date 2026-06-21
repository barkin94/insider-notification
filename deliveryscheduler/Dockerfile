FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY deliveryscheduler/go.mod deliveryscheduler/go.sum deliveryscheduler/
COPY shared/go.mod shared/go.sum shared/
RUN cd deliveryscheduler && go mod download

COPY deliveryscheduler/ deliveryscheduler/
COPY shared/ shared/

RUN cd deliveryscheduler && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /deliveryscheduler-server ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=builder /deliveryscheduler-server /deliveryscheduler-server
ENTRYPOINT ["/deliveryscheduler-server"]
