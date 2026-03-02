#!/bin/bash
set -euo pipefail

IMAGE="${1:?usage: build-rootfs.sh <docker-image> [output] [size]}"
OUTPUT="${2:-build/rootfs.ext4}"
SIZE="${3:-512M}"

export_image() {
  cid=""
  trap 'if [ -n "${cid-}" ]; then docker rm -f "${cid}"; fi; rm -rf build/rootfs-staging' EXIT
  rm -rf build/rootfs-staging
  cid=$(docker create "${IMAGE}" /bin/true)
  mkdir -p build/rootfs-staging
  docker export "${cid}" | tar -xf - -C build/rootfs-staging
}

build() {
  mkdir -p "$(dirname "${OUTPUT}")"
  truncate -s "${SIZE}" "${OUTPUT}"
  mkfs.ext4 -q -d build/rootfs-staging "${OUTPUT}"
}

main() {
  export_image
  build
}

main
