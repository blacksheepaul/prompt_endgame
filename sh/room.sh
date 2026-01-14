#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8081}"

# --- preflight ---
command -v jq >/dev/null 2>&1 || {
  echo "error: jq is required but not installed" >&2
  echo "install: brew install jq / apt install jq" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
room - local CLI

Usage:
  room create
  room watch <room_id>
  room say <room_id> <user_input>
  room help

Env:
  BASE_URL   default: http://127.0.0.1:8081

Examples:
  rid="$(room create)"
  room watch "$rid"
  room say "$rid" "hello world"
EOF
}

die() {
  echo "error: $*" >&2
  exit 1
}

# --- commands ---

cmd_create() {
  [[ $# -eq 0 ]] || die "usage: room create"

  curl -sS -X POST "$BASE_URL/rooms" \
  | jq -r '.id'
}

cmd_watch() {
  local rid="${1:-}"
  [[ -n "$rid" ]] || die "usage: room watch <room_id>"

  exec curl -N -sS "$BASE_URL/rooms/$rid/events"
}

cmd_say() {
  local rid="${1:-}"
  local input="${2:-}"

  [[ -n "$rid" && -n "$input" ]] \
    || die "usage: room say <room_id> <user_input>"

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
