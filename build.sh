#!/usr/bin/env bash
set -euo pipefail

mkdir -p bin

apps=(
  "dnslog:./cmd/dnslog"
  "arpscout:./cmd/arpscout"
  "netatlas:./cmd/netatlas"
)

targets=(
  "linux:amd64:native"
  "linux:arm64:arm64"
  "linux:arm:arm32"
)

for target in "${targets[@]}"; do
  IFS=":" read -r goos goarch suffix <<< "$target"

  for app in "${apps[@]}"; do
    IFS=":" read -r name path <<< "$app"

    out="bin/${name}-${suffix}"

    echo "Building ${name} for ${goos}/${goarch} -> ${out}"

    if [[ "$goarch" == "arm" ]]; then
      GOOS="$goos" GOARCH="$goarch" GOARM=7 go build -o "$out" "$path"
    else
      GOOS="$goos" GOARCH="$goarch" go build -o "$out" "$path"
    fi
  done
done

echo "Build complete."