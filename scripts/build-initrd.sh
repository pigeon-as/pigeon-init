#!/bin/bash
set -euo pipefail

CONFIG="${1:-}"
INIT_BIN="out/init"

stage() {
  rm -rf out/initrd-staging
  mkdir -p out/initrd-staging/{dev,pigeon,newroot}
  cp "${INIT_BIN}" out/initrd-staging/init
  chmod 755 out/initrd-staging/init
  if [[ -n "${CONFIG}" && -f "${CONFIG}" ]]; then
    cp "${CONFIG}" out/initrd-staging/pigeon/run.json
  fi
}

pack() {
  (cd out/initrd-staging && find . | cpio --quiet -o -H newc) > out/initrd.cpio
  rm -rf out/initrd-staging
}

main() {
  [[ -f "${INIT_BIN}" ]] || { echo "init binary not found — run 'make init' first" >&2; exit 1; }
  stage
  pack
}

main
