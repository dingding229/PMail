# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:22.23.5-alpine3.24 AS frontend
WORKDIR /build/fe
COPY fe/package.json fe/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts --no-audit --no-fund
COPY fe/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS backend
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev
ARG GITHASH=unknown
WORKDIR /build/server
COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY server/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    export CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH"; \
    if [ "$GOARCH" = arm ] && [ -n "$TARGETVARIANT" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    mkdir -p /out; \
    go build -trimpath -ldflags "-s -w -X 'main.version=$VERSION' -X 'main.goVersion=$(go version)' -X 'main.gitHash=$GITHASH' -X 'main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" -o /out/pmail .

FROM alpine:3.24.4
WORKDIR /work
RUN apk add --no-cache ca-certificates tzdata \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo Asia/Shanghai > /etc/timezone \
    && mkdir -m 700 /work/config
COPY --from=backend /out/pmail /work/pmail
COPY --from=frontend /build/fe/dist/ /work/web/
VOLUME ["/work/config"]
EXPOSE 25 80 110 143 443 465 587 993 995
HEALTHCHECK --interval=30s --timeout=5s --start-period=75s --retries=3 CMD ["/work/pmail", "healthcheck"]
CMD ["/work/pmail"]
