# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:22.23.5-alpine3.24 AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts --no-audit --no-fund
COPY web/ ./
RUN VITE_MOCK=0 npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS backend
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=0.0.6
ARG GITHASH=unknown
ARG BUILD_TIME
WORKDIR /build/server
COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY server/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    case "$VERSION" in *[!0-9.]*|'') echo "Invalid PMail Pro version" >&2; exit 1;; esac; \
    printf '%s\n' "$VERSION" | grep -Eq '^0\.[0-9]+\.[0-9]+$' || { echo "Invalid PMail Pro version" >&2; exit 1; }; \
    export CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH"; \
    if [ "$GOARCH" = arm ] && [ -n "$TARGETVARIANT" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    mkdir -p /out; \
    build_time="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"; \
    go build -trimpath -ldflags "-s -w -X 'main.version=$VERSION' -X 'main.goVersion=$(go version)' -X 'main.gitHash=$GITHASH' -X 'main.buildTime=$build_time'" -o /out/pmail .

FROM scratch AS release-files
COPY --from=backend /out/pmail /pmail
COPY --from=frontend /build/web/dist/ /web/

FROM alpine:3.24.4
WORKDIR /work
RUN apk add --no-cache ca-certificates tzdata \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo Asia/Shanghai > /etc/timezone \
    && mkdir -m 700 /work/config
COPY --from=release-files / /work/
EXPOSE 25 80 110 143 443 465 587 993 995
HEALTHCHECK --interval=30s --timeout=5s --start-period=75s --retries=3 CMD ["/work/pmail", "healthcheck"]
CMD ["/work/pmail"]
