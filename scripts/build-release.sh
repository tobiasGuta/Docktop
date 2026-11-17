#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 2 || ! $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$ ]]; then
    printf 'Usage: %s vMAJOR.MINOR.PATCH OUTPUT_DIRECTORY\n' "$0" >&2
    exit 2
fi

version=$1
mkdir -p "$2"
output=$(cd "$2" && pwd)
cd "$(dirname "$0")/.."

staging=$(mktemp -d)
cleanup() {
    if [[ -f "$staging/docktop" ]]; then
        unlink "$staging/docktop"
    fi
    rmdir "$staging"
}
trap cleanup EXIT

archives=()
for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${platform%/*}
    arch=${platform#*/}
    archive="docktop_${version}_${os}_${arch}.tar.gz"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -mod=readonly -trimpath -buildvcs=false -ldflags "-X main.version=$version" -o "$staging/docktop" ./cmd/docktop
    tar -czf "$output/$archive" -C "$staging" docktop -C "$PWD" README.md docs/dashboard.png
    archives+=("$archive")
done

cd "$output"
sha256sum "${archives[@]}" > SHA256SUMS
