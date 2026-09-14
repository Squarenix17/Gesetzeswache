# syntax=docker/dockerfile:1
#
# Digests pinned as of 2026-09-14. To update, re-resolve via
#   docker buildx imagetools inspect <image>:<tag>
# (or crane/skopeo), update both tag and digest, rebuild.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:ce864e7223ac17b1775e6fd0b4c0db580c2eb50e7953a427916379e4b92a1628 AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY VERSION ./
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN set -e; \
    V="${VERSION:-$(tr -d '[:space:]' < VERSION)}"; \
    GOOS="${TARGETOS:-linux}"; \
    GOARCH="${TARGETARCH:-$(go env GOARCH)}"; \
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="-s -w -X main.version=${V}" -o /out/gew ./cmd/gesetzeswache

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
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
