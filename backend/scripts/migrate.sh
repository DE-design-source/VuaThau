#!/usr/bin/env bash
# Apply all SQL migrations in migrations/ (sorted by filename) using psql.
#
# Usage:
#   DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable scripts/migrate.sh
#   scripts/migrate.sh            # reads DATABASE_URL from backend/.env if not set
#
# Migrations are written to be idempotent, so re-running this script is safe.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${ROOT_DIR}/migrations"
ENV_FILE="${ROOT_DIR}/.env"

# Load KEY=VALUE pairs from .env without overriding variables already set in
# the environment. Parsed line by line (not sourced) so arbitrary .env
# content can't execute as shell code.
load_env() {
  local file="$1" line key value
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"                              # tolerate CRLF files
    [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
    line="${line#"${line%%[![:space:]]*}"}"           # trim leading space
    line="${line#export }"
    [[ "$line" == *=* ]] || continue
    key="${line%%=*}"
    key="${key%"${key##*[![:space:]]}"}"              # trim trailing space
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

if ! command -v psql >/dev/null 2>&1; then
  echo "error: psql not found in PATH (install the PostgreSQL client)" >&2
  exit 1
fi

shopt -s nullglob
files=("${MIGRATIONS_DIR}"/*.sql)
shopt -u nullglob

if (( ${#files[@]} == 0 )); then
  echo "no migrations found in ${MIGRATIONS_DIR}"
  exit 0
fi

# Glob expansion is already sorted, but sort explicitly with a fixed locale.
IFS=$'\n' read -r -d '' -a files < <(printf '%s\n' "${files[@]}" | LC_ALL=C sort && printf '\0')

for f in "${files[@]}"; do
  echo "==> applying $(basename "$f")"
  psql "$DATABASE_URL" \
    --no-psqlrc \
    --quiet \
    --single-transaction \
    -v ON_ERROR_STOP=1 \
    -f "$f"
done

echo "migrations applied: ${#files[@]}"
