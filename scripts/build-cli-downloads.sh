#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUTPUT=${1:-"$ROOT/apps/web/public/downloads/cli"}
VERSION=${VERSION:-dev}
mkdir -p "$OUTPUT"
OUTPUT=$(CDPATH= cd -- "$OUTPUT" && pwd)
TEMP=$(mktemp -d)
trap 'rm -rf "$TEMP"' EXIT HUP INT TERM

cd "$ROOT/backend"
for os in darwin linux windows; do
  for arch in amd64 arm64; do
    binary=anby-wiki
    if [ "$os" = windows ]; then binary=anby-wiki.exe; fi
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
      -ldflags="-s -w -X main.version=$VERSION" -o "$TEMP/$binary" ./cmd/anby-wiki
    COPYFILE_DISABLE=1 tar -czf "$OUTPUT/anby-wiki-$os-$arch.tar.gz" -C "$TEMP" "$binary"
    # Verify the archive before exposing it as a download.
    [ "$(tar -tzf "$OUTPUT/anby-wiki-$os-$arch.tar.gz")" = "$binary" ]
    rm "$TEMP/$binary"
  done
done
cd "$OUTPUT"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum anby-wiki-*.tar.gz > SHA256SUMS.txt
else
  shasum -a 256 anby-wiki-*.tar.gz > SHA256SUMS.txt
fi
printf '%s\n' "$VERSION" > VERSION.txt
