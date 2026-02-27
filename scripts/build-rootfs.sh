#!/bin/bash
set -euo pipefail

IMAGE="${1:?usage: build-rootfs.sh <docker-image> [size]}"
OUTPUT="${2:-out/rootfs.ext4}"
SIZE="${3:-512M}"

export_image() {
  local cid
  cid=$(docker create "${IMAGE}" /bin/true)
  trap 'docker rm -f "${cid}" >/dev/null 2>&1; rm -rf out/rootfs-staging' EXIT
  rm -rf out/rootfs-staging
  mkdir -p out/rootfs-staging
  docker export "${cid}" | tar -xf - -C out/rootfs-staging
}

build() {
  mkdir -p "$(dirname "${OUTPUT}")"
  truncate -s "${SIZE}" "${OUTPUT}"
  mkfs.ext4 -q -d out/rootfs-staging "${OUTPUT}"
}

main() {
  export_image
  build
}

main
