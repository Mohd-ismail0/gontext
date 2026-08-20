# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.5
ARG TARGETOS=linux
ARG TARGETARCH=amd64

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src

COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download || true

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
    -o /out/context-fabric ./cmd/context-fabric

FROM alpine:3.21 AS runtime
ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache ca-certificates wget \
  && addgroup -g 65532 -S nonroot \
  && adduser -u 65532 -S nonroot -G nonroot

COPY --from=builder /out/context-fabric /usr/local/bin/context-fabric

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/context-fabric"]
# Role selected via CMD: serve | worker | connector | migrate | bootstrap | doctor | backup | restore | reconcile | all
CMD ["serve"]
