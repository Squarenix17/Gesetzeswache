# syntax=docker/dockerfile:1
FROM golang:1.21-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/gew ./cmd/gesetzeswache

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/gew /gew
COPY variants /variants
ENV GEW_VARIANTS_PATH=/variants/variants.tsv
ENV GEW_LINKED_INSTRUMENTS_PATH=/variants/linked_instruments.tsv
ENV GEW_STORE_PATH=/tmp/gesetzeswache.db
ENV GEW_DISCOVERY_ENABLED=true
ENV GEW_DISCOVERY_MAX_PER_CYCLE=50
ENV GEW_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gew"]
CMD ["serve"]
