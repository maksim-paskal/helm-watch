FROM golang:1.22-alpine as builder
WORKDIR /app
COPY . .

ENV CGO_ENABLED=0
RUN go build -ldflags "-s -w" -o helm-watch ./cmd

FROM alpine:latest
COPY --from=builder /app/helm-watch /helm-watch
ENTRYPOINT ["/helm-watch"]