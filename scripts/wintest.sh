#!/usr/bin/env bash
# Cross-compile a package's tests for Windows and run them on a Windows host
# over SSH. The host needs OpenSSH server with PowerShell as its shell.
#
# Usage: WINTEST_HOST=<ssh host> scripts/wintest.sh <package> [test-regex]
set -euo pipefail

host=${WINTEST_HOST:?set WINTEST_HOST to the ssh host of a Windows machine}
pkg=${1:?usage: scripts/wintest.sh <package> [test-regex]}
run=${2:-}

name=$(basename "$pkg")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c -o "$tmp/$name.test.exe" "$pkg"

ssh -o BatchMode=yes "$host" 'New-Item -ItemType Directory -Force kite-test | Out-Null; Remove-Item -Recurse -Force kite-test\* -ErrorAction SilentlyContinue' >/dev/null
scp -q "$tmp/$name.test.exe" "$host:kite-test/"
dir=$(go list -f '{{.Dir}}' "$pkg")
if [ -d "$dir/testdata" ]; then
  # scp cannot create directories on Windows; ship one archive and unpack it
  # with the tar.exe that Windows 10+ includes.
  tar -czf "$tmp/testdata.tgz" -C "$dir" testdata
  scp -q "$tmp/testdata.tgz" "$host:kite-test/"
  ssh -o BatchMode=yes "$host" 'Set-Location kite-test; tar -xzf testdata.tgz' >/dev/null
fi

# Quote each flag whole: PowerShell 5.1 splits bare "-test.run" at the dot.
args="'-test.v'"
if [ -n "$run" ]; then
  args="$args '-test.run=$run'"
fi
ssh -o BatchMode=yes "$host" "Set-Location kite-test; & .\\$name.test.exe $args; exit \$LASTEXITCODE"
