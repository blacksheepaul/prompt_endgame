#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8081}"
ALIAS_DIR="${HOME}/.prompt_endgame"

# --- preflight ---
command -v jq >/dev/null 2>&1 || {
  echo "error: jq is required but not installed" >&2
  echo "install: brew install jq / apt install jq" >&2
  exit 1
}

init_alias_dir() {
  if [[ ! -d "$ALIAS_DIR" ]]; then
    mkdir -p "$ALIAS_DIR"
  fi
}

validate_alias() {
  local alias="${1:-}"
  if [[ ! "$alias" =~ ^[a-z0-9]+$ ]]; then
    die "alias must only contain lowercase letters and numbers (a-z, 0-9)"
  fi
}

resolve_alias() {
  local id_or_alias="${1:-}"

  # Check if it's an alias (exists as file)
  local alias_file="$ALIAS_DIR/$id_or_alias"
  if [[ -f "$alias_file" ]]; then
    cat "$alias_file"
    return
  fi

  # Otherwise return as is (room_id)
  echo "$id_or_alias"
}

usage() {
  cat >&2 <<'EOF'
room - local CLI

Usage:
  room create [alias]
  room watch <room_id|alias>
  room say <room_id|alias> <user_input>
  room help

Env:
  BASE_URL   default: http://127.0.0.1:8081

Alias rules:
  Alias must only contain lowercase letters and numbers (a-z, 0-9)

Examples:
  rid="$(room create)"
  room watch "$rid"
  room say "$rid" "hello world"

With aliases:
  room create foo
  room watch foo
  room say foo "hello world"
EOF
}

die() {
  echo "error: $*" >&2
  exit 1
}

# --- commands ---

cmd_create() {
  local alias="${1:-}"
  [[ $# -le 1 ]] || die "usage: room create [alias]"

  # Validate alias if provided
  if [[ -n "$alias" ]]; then
    validate_alias "$alias"
  fi

  local rid
  rid=$(curl -sS -X POST "$BASE_URL/rooms" | jq -r '.id')

  # Save to alias file if alias provided
  if [[ -n "$alias" ]]; then
    init_alias_dir
    echo "$rid" > "$ALIAS_DIR/$alias"
    echo "Created room: $rid (saved as alias: $alias)"
  else
    echo "$rid"
  fi
}

cmd_watch() {
  local id_or_alias="${1:-}"
  [[ -n "$id_or_alias" ]] || die "usage: room watch <room_id|alias>"

  local rid
  rid=$(resolve_alias "$id_or_alias")

  exec curl -N -sS "$BASE_URL/rooms/$rid/events"
}

cmd_say() {
  local id_or_alias="${1:-}"
  local input="${2:-}"

  [[ -n "$id_or_alias" && -n "$input" ]] \
    || die "usage: room say <room_id|alias> <user_input>"

  local rid
  rid=$(resolve_alias "$id_or_alias")

  curl -sS -X POST \
    "$BASE_URL/rooms/$rid/answer" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg v "$input" '{user_input:$v}')"

  echo
}

# --- entry ---

main() {
  local cmd="${1:-help}"
  shift || true

  case "$cmd" in
    create) cmd_create "$@" ;;
    watch)  cmd_watch "$@" ;;
    say)    cmd_say "$@" ;;
    help|-h|--help) usage ;;
    *)
      usage
      die "unknown command: $cmd"
      ;;
  esac
}

main "$@"
