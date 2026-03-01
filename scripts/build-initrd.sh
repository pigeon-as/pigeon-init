#!/bin/bash
set -euo pipefail

CONFIG="${1:-}"
INIT_BIN="out/init"
ROOT="out/initrd-root"

[[ -f "${INIT_BIN}" ]] || { echo "error: run 'make build' first" >&2; exit 1; }

rm -rf "${ROOT}"
trap 'rm -rf "${ROOT}"' EXIT

mkdir -p "${ROOT}"/{dev,pigeon,newroot}
cp "${INIT_BIN}" "${ROOT}/init"
chmod 755 "${ROOT}/init"

[[ -n "${CONFIG}" && -f "${CONFIG}" ]] && cp "${CONFIG}" "${ROOT}/pigeon/run.json"

mkdir -p out
(cd "${ROOT}" && find . | cpio --quiet -o -H newc) > out/initrd.cpio
