FROM golang:1.25-alpine AS build

WORKDIR /src
COPY bff/go.mod bff/go.sum ./
RUN go mod download
COPY bff ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/infinitemail-bff ./cmd/bff

FROM alpine:3.22

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/infinitemail-bff /app/infinitemail-bff
COPY bff/migrations /app/migrations
RUN mkdir -p /data/attachments && chown -R app:app /data /app
USER app

ENV HTTP_ADDR=:1666
ENV MIGRATIONS_DIR=/app/migrations
ENV DATA_PATH=/data/infinitemail-bff.json
ENV ATTACHMENT_DIR=/data/attachments
EXPOSE 1666

CMD ["/app/infinitemail-bff"]
