#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC="${ROOT}/../api/openapi/colony2-api.yaml"
OUT="${ROOT}/internal/openapi/generated.go"
CONFIG="${ROOT}/openapi.codegen.yaml"

if [[ ! -f "${SPEC}" ]]; then
  echo "Spec not found: ${SPEC}" >&2
  exit 1
fi

go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.1 \
  -config "${CONFIG}" \
  -o "${OUT}" \
  "${SPEC}"

gofmt -w "${OUT}"
echo "Regenerated ${OUT}"
