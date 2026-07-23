# syntax=docker/dockerfile:1
FROM golang:1.21-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/gesetzeswache ./cmd/gesetzeswache

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/gesetzeswache /gesetzeswache
COPY variants /variants
ENV GEW_VARIANTS_PATH=/variants/variants.tsv
ENV GEW_STORE_PATH=/tmp/gesetzeswache.db
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gesetzeswache"]
CMD ["serve"]
