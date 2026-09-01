# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.5
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
WORKDIR /src

COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download || true

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X main.buildVersion=${VERSION}" \
    -o /out/context-fabric ./cmd/context-fabric

FROM alpine:3.21 AS runtime
ARG TARGETOS
ARG TARGETARCH
ARG VERSION

LABEL org.opencontainers.image.title="gontext" \
      org.opencontainers.image.description="Context Fabric — governed organizational context plane" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/mohd-ismail0/gontext"

RUN apk add --no-cache ca-certificates wget \
  && addgroup -g 65532 -S nonroot \
  && adduser -u 65532 -S nonroot -G nonroot

COPY --from=builder /out/context-fabric /usr/local/bin/context-fabric
COPY internal/migrate/migrations /migrations
COPY contracts/openfga /contracts/openfga
COPY scripts /scripts
ENV MIGRATIONS_DIR=/migrations
ENV OPENFGA_MODEL_PATH=/contracts/openfga/model.fga
ENV OPENFGA_MODEL_JSON=/contracts/openfga/model.json
ENV CONTEXT_FABRIC_VERSION=${VERSION}
# Ops roles (backup|restore|reconcile) invoke /scripts/*.sh when present.
# Shell scripts require ash/bash on the host or a sidecar; the runtime image
# ships the scripts for operator bind-mount / kubectl cp convenience.

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/context-fabric"]
# Role selected via CMD: serve | worker | connector | migrate | bootstrap | doctor | backup | restore | reconcile | all
CMD ["serve"]
