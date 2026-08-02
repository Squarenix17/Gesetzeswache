# syntax=docker/dockerfile:1
#
# Digests pinned as of 2026-07-29. To update, re-resolve via
#   docker buildx imagetools inspect <image>:<tag>
# (or crane/skopeo), update both tag and digest, rebuild.
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.5.1
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/gew ./cmd/gesetzeswache

FROM gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35
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
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s CMD ["/gew", "health"]
USER nonroot:nonroot
ENTRYPOINT ["/gew"]
CMD ["serve"]
