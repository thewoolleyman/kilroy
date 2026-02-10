#!/usr/bin/env bash
set -euo pipefail

UI_URL="${KILROY_CXDB_UI_URL:-${KILROY_CXDB_HTTP_BASE_URL:-${CXDB_HTTP_BASE_URL:-http://127.0.0.1:9010}}}"

echo "cxdb_ui=$UI_URL"

if [[ "${KILROY_CXDB_OPEN_UI:-0}" == "1" ]]; then
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$UI_URL" >/dev/null 2>&1 || true
  elif command -v open >/dev/null 2>&1; then
    open "$UI_URL" >/dev/null 2>&1 || true
  fi
fi
