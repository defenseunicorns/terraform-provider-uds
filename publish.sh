#!/usr/bin/env bash

set -euo pipefail

cd dist

layout="tmp-layout"

cleanup() {
    rm -rf tmp-layout
}

trap cleanup EXIT

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
name=$(jq -r .project_name metadata.json)
version=$(jq -r .version metadata.json)
index_to_tag="${name}_$version.json"

jq --arg mediatype "$index_mediatype" '
  .artifactType = $mediatype |
  .manifests = (.manifests | map(
    if .annotations."org.opencontainers.image.ref.name" then
      . + {
        platform: {
          os: (.annotations."org.opencontainers.image.ref.name" | split("_")[0]),
          architecture: (.annotations."org.opencontainers.image.ref.name" | split("_")[1])
        }
      }
    else
      .
    end
  ))
' "$index" > "$index_to_tag"

oras manifest push --oci-layout "$layout:$version" "$index_to_tag"

go tool oras cp \
  --from-oci-layout "$layout:$version" \
  "registry.defenseunicorns.com/ops/$name:$version"
