#!/usr/bin/env bash

set -euo pipefail

cd dist

# EXACTLY 1 descriptor whose mediatype: archive/zip

layout="tmp-layout"

cleanup() {
    rm -rf tmp-layout
}

# trap cleanup EXIT

version=$(jq -r .version metadata.json)

mkdir "$layout"

index_mediatype="application/vnd.opentofu.provider"
manifest_mediatype="application/vnd.opentofu.provider-target"
layer_mediatype="archive/zip"

find . -name "*.zip" -printf "%f\n" | while read -r archive; do
    entry=$(jq ".[] | select(.name == \"${archive}\")" artifacts.json)
    os_arch=$(echo "$entry" | jq -r '"\(.goos)_\(.goarch)"')

    go tool oras push \
      --artifact-type "$manifest_mediatype" \
      --oci-layout "$layout:$os_arch" \
      "$archive:$layer_mediatype"
done

index="$layout/index.json"
tmp_index="$index.tmp"
cp "$index" "$tmp_index"

jq ".artifactType = \"${index_mediatype}\"" "$tmp_index"
