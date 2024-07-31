FROM alpine:latest
COPY ./helm-watch /helm-watch
ENTRYPOINT ["/helm-watch"]