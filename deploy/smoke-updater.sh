#!/usr/bin/env bash
# Test the image's real HTTP mux. Deliberately do not mount Docker's socket
# or any Compose project: the background job may fail, but POST must be 202.
set -euo pipefail
image=${1:?updater image tag required}
name=${2:?temporary container name required}
token=$(openssl rand -hex 32)
body=$(mktemp)
container=
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
  rm -f "$body"
}
trap cleanup EXIT
container=$(docker run -d --name "$name" --cpus 1.5 --memory 1g \
  --tmpfs /state:rw,mode=0700 -p 127.0.0.1::8989 \
  -e PMAIL_UPDATER_TOKEN="$token" -e PMAIL_UPDATER_PROJECT=pmail-c11-smoke "$image")
address=$(docker port "$container" 8989/tcp)
url="http://$address/jobs"
curl --silent --show-error --retry 10 --retry-connrefused --retry-delay 1 \
  --max-time 4 --output /dev/null "$url"
code=$(curl --silent --show-error --max-time 4 --output "$body" --write-out '%{http_code}' \
  -H "Authorization: Bearer $token" -X POST "$url")
if [[ "$code" != 202 ]]; then
  printf 'Expected HTTP 202 for POST /jobs; got %s\n' "$code" >&2
  cat "$body" >&2
  docker logs "$container" >&2
  exit 1
fi
printf 'POST /jobs => HTTP %s\n' "$code"
