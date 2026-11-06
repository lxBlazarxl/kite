#!/usr/bin/env bash
# Builds a release into dist/: the three agents (also embedded into the hub
# through internal/agentdl/bin), the hub for linux/amd64 and linux/arm64,
# and SHA256SUMS. Used by the release workflow and runnable locally.
#
# Usage: scripts/release-build.sh <version>
set -euo pipefail

version=${1:?usage: scripts/release-build.sh <version>}
cd "$(dirname "$0")/.."

ldflags="-s -w -X github.com/retransmit/kite/internal/version.Version=$version"
build() { # goos goarch package output
	CGO_ENABLED=0 GOOS=$1 GOARCH=$2 go build -trimpath -ldflags "$ldflags" -o "$4" "$3"
}

rm -rf dist
rm -f internal/agentdl/bin/kite-agent-*
mkdir -p dist

build linux amd64 ./cmd/kite-agent internal/agentdl/bin/kite-agent-linux-amd64
build linux arm64 ./cmd/kite-agent internal/agentdl/bin/kite-agent-linux-arm64
build windows amd64 ./cmd/kite-agent internal/agentdl/bin/kite-agent-windows-amd64.exe
cp internal/agentdl/bin/kite-agent-* dist/

# The hub embeds the agents built above.
build linux amd64 ./cmd/kite-hub dist/kite-hub-linux-amd64
build linux arm64 ./cmd/kite-hub dist/kite-hub-linux-arm64

(cd dist && sha256sum kite-* | sort -k2 >SHA256SUMS)
echo "built $version:"
ls -l dist
