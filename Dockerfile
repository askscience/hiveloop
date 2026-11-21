# Hiveloop: single static binary, data in /data.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /hiveloop .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 hiveloop && mkdir /data && chown hiveloop /data
COPY --from=build /hiveloop /usr/local/bin/hiveloop
USER hiveloop
ENV HIVELOOP_ADDR=0.0.0.0:8090 HIVELOOP_DATA=/data
VOLUME /data
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/hiveloop"]
