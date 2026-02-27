#!/bin/bash
set -euo pipefail

IMAGE="${1:?usage: build-rootfs.sh <docker-image> [size]}"
OUTPUT="${2:-out/rootfs.ext4}"
SIZE="${3:-512M}"

export_image() {
  cid=""
  trap 'if [ -n "${cid-}" ]; then docker rm -f "${cid}"; fi; rm -rf out/rootfs-staging' EXIT
  rm -rf out/rootfs-staging
  cid=$(docker create "${IMAGE}" /bin/true)
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
