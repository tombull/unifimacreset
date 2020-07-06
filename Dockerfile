FROM golang:latest as builder

RUN mkdir /app
WORKDIR /app

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build

FROM alpine:latest
RUN apk update && apk add ca-certificates

RUN mkdir /app
WORKDIR /app
COPY --from=builder /app/unifimacreset /app

EXPOSE 9000

CMD ./unifimacreset
