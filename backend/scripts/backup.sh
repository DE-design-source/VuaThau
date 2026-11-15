#!/usr/bin/env bash
# Dump the database to backups/<timestamp>.sql.gz using pg_dump.
#
# Usage:
#   DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable scripts/backup.sh
#   scripts/backup.sh             # reads DATABASE_URL from backend/.env if not set
#
# Restore:
#   gunzip -c backups/<timestamp>.sql.gz | psql "$DATABASE_URL" -v ON_ERROR_STOP=1
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
BACKUP_DIR="${BACKUP_DIR:-${ROOT_DIR}/backups}"
ENV_FILE="${ROOT_DIR}/.env"

# Same .env loader as migrate.sh: parse, don't source; never override the
# existing environment.
load_env() {
  local file="$1" line key value
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line#export }"
    [[ "$line" == *=* ]] || continue
    key="${line%%=*}"
    key="${key%"${key##*[![:space:]]}"}"
    value="${line#*=}"
    value="${value#"${value%%[![:space:]]*}"}"
    if [[ "$value" =~ ^\"(.*)\"$ || "$value" =~ ^\'(.*)\'$ ]]; then
      value="${BASH_REMATCH[1]}"
    fi
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ -z "${!key+x}" ]]; then
      export "$key=$value"
    fi
  done < "$file"
}

load_env "$ENV_FILE"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "error: DATABASE_URL is not set (export it or define it in ${ENV_FILE})" >&2
  exit 1
fi

for bin in pg_dump gzip; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: ${bin} not found in PATH" >&2
    exit 1
  fi
done

mkdir -p "$BACKUP_DIR"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="${BACKUP_DIR}/${timestamp}.sql.gz"
tmp="${out}.partial"

# Remove the partial file if anything fails (pipefail catches pg_dump errors).
trap 'rm -f "$tmp"' EXIT

echo "==> dumping database to ${out}"
pg_dump "$DATABASE_URL" --no-owner --no-privileges | gzip -9 > "$tmp"
mv "$tmp" "$out"

echo "backup written: ${out} ($(du -h "$out" | cut -f1))"
