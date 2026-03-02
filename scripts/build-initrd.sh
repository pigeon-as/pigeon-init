#!/bin/bash
set -euo pipefail

OUTPUT="${1:-build/initrd.cpio}"
CONFIG="${2:-}"
INIT_BIN="build/init"
ROOT="build/initrd-root"

[[ -f "${INIT_BIN}" ]] || { echo "error: run 'make build' first" >&2; exit 1; }

rm -rf "${ROOT}"
trap 'rm -rf "${ROOT}"' EXIT

mkdir -p "${ROOT}"/{dev,pigeon,newroot}
cp "${INIT_BIN}" "${ROOT}/init"
chmod 755 "${ROOT}/init"

[[ -n "${CONFIG}" && -f "${CONFIG}" ]] && cp "${CONFIG}" "${ROOT}/pigeon/run.json"

mkdir -p "$(dirname "${OUTPUT}")"
(cd "${ROOT}" && find . | cpio --quiet -o -H newc) > "${OUTPUT}"
