#!/usr/bin/env bash
# run_stage_b.sh — execute the full Stage B load test matrix
#
# Usage:
#   ./scripts/run_stage_b.sh [--dry-run]
#
# Requires:
#   - prompt_endgame server running at http://localhost:10180
#   - Fake LLM server running at http://localhost:10181

set -euo pipefail

ENDGAME_URL="${ENDGAME_URL:-http://localhost:10180}"
FAKE_LLM_URL="${FAKE_LLM_URL:-http://localhost:10181}"
DURATION="${DURATION:-60s}"
DRY_RUN=false

# Test matrix: "concurrency:scenario"
TEST_MATRIX=(
  "10:fast"
  "10:slow"
  "50:fast"
  "50:backpressure"
  "100:fast"
)

# ─── helpers ────────────────────────────────────────────────────────────────

info()  { printf '\033[1;34m[INFO]\033[0m  %s\n' "$*"; }
ok()    { printf '\033[1;32m[OK]\033[0m    %s\n' "$*"; }
warn()  { printf '\033[1;33m[WARN]\033[0m  %s\n' "$*"; }
die()   { printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; exit 1; }

check_service() {
  local name="$1"
  local url="$2"
  local path="${3:-/}"
  if curl -sf --max-time 3 "${url}${path}" > /dev/null 2>&1; then
    ok "${name} is reachable at ${url}"
  else
    die "${name} not reachable at ${url}. Please start it first."
  fi
}

run_test() {
  local concurrency="$1"
  local scenario="$2"
  local timestamp
  timestamp=$(date +%Y%m%d_%H%M%S)
  local out_dir="benchmarks/stageb_${timestamp}_${concurrency}r_${scenario}"

  info "Running: concurrency=${concurrency} scenario=${scenario} duration=${DURATION}"
  info "Output: ${out_dir}"

  if $DRY_RUN; then
    warn "DRY RUN — skipping actual test"
    return 0
  fi

  go run ./scripts/baseline_loadtest.go \
    -c "${concurrency}" \
    -d "${DURATION}" \
    -o "${out_dir}" \
    --scenario "${scenario}" \
    --fake-llm "${FAKE_LLM_URL}"

  ok "Completed: ${out_dir}"
  echo "${out_dir}"
}

# ─── argument parsing ────────────────────────────────────────────────────────

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    *) die "Unknown argument: ${arg}" ;;
  esac
done

# ─── pre-flight checks ───────────────────────────────────────────────────────

info "Stage B Load Test Suite"
info "Endgame URL : ${ENDGAME_URL}"
info "Fake LLM URL: ${FAKE_LLM_URL}"
info "Duration    : ${DURATION}"
info "Matrix      : ${TEST_MATRIX[*]}"
echo

check_service "prompt_endgame" "${ENDGAME_URL}" "/metrics"
check_service "Fake LLM"       "${FAKE_LLM_URL}" "/admin/config"
echo

# ─── run matrix ─────────────────────────────────────────────────────────────

SUMMARY_DIR="benchmarks/stageb_summary_$(date +%Y%m%d_%H%M%S)"
mkdir -p "${SUMMARY_DIR}"
SUMMARY_FILE="${SUMMARY_DIR}/summary.md"

{
  echo "# Stage B Load Test — Summary"
  echo
  echo "Generated: $(date '+%Y-%m-%d %H:%M:%S')"
  echo
  echo "| Run | Concurrency | Scenario | Report |"
  echo "|-----|-------------|----------|--------|"
} > "${SUMMARY_FILE}"

run_num=0
for entry in "${TEST_MATRIX[@]}"; do
  run_num=$((run_num + 1))
  concurrency="${entry%%:*}"
  scenario="${entry##*:}"

  out_dir=$(run_test "${concurrency}" "${scenario}")

  # Find the report file produced by baseline_loadtest.go
  report_path=""
  if [[ -n "${out_dir}" && -d "${out_dir}" ]]; then
    report_path=$(find "${out_dir}" -name "report_*.md" | head -1 || true)
  fi

  echo "| ${run_num} | ${concurrency} | ${scenario} | ${report_path:-n/a} |" >> "${SUMMARY_FILE}"
  echo
done

info "All runs complete."
ok  "Summary report: ${SUMMARY_FILE}"
cat "${SUMMARY_FILE}"
