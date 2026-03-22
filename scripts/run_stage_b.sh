#!/usr/bin/env bash

set -euo pipefail

ENDGAME_URL="${ENDGAME_URL:-http://localhost:10180}"
PROFILE_DIR="${PROFILE_DIR:-benchmarks/profiles/stageb_v1}"
DRY_RUN=false

PROFILE_MATRIX=(
  "10_fast.json"
  "50_fast.json"
  "50_slow.json"
  "100_fast.json"
)

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    *)
      printf '[ERROR] Unknown argument: %s\n' "$arg" >&2
      exit 1
      ;;
  esac
done

if [[ ! -d "${PROFILE_DIR}" ]]; then
  printf '[ERROR] Profile directory does not exist: %s\n' "${PROFILE_DIR}" >&2
  exit 1
fi

summary_dir="benchmarks/stageb_summary_$(date +%Y%m%d_%H%M%S)"
mkdir -p "${summary_dir}"
summary_file="${summary_dir}/summary.md"

{
  echo "# Stage B Load Test — Summary"
  echo
  echo "Generated: $(date '+%Y-%m-%d %H:%M:%S')"
  echo
  echo "| Run | Profile | Report |"
  echo "|-----|---------|--------|"
} > "${summary_file}"

run_num=0
for profile_file in "${PROFILE_MATRIX[@]}"; do
  run_num=$((run_num + 1))
  profile_path="${PROFILE_DIR}/${profile_file}"

  if [[ ! -f "${profile_path}" ]]; then
    printf '[ERROR] Missing profile file: %s\n' "${profile_path}" >&2
    exit 1
  fi

  out_dir="benchmarks/stageb_$(date +%Y%m%d_%H%M%S)_${profile_file%.json}"
  cmd=(go run ./scripts/baseline_loadtest.go --base-url "${ENDGAME_URL}" --profile "${profile_path}" -o "${out_dir}")

  printf '[INFO] Run %d profile=%s\n' "${run_num}" "${profile_file}"
  if $DRY_RUN; then
    printf '[DRY] %s\n' "${cmd[*]}"
    report_path="n/a"
  else
    "${cmd[@]}"
    report_path="n/a"
    for candidate in "${out_dir}"/report_*.md; do
      if [[ -f "${candidate}" ]]; then
        report_path="${candidate}"
        break
      fi
    done
  fi

  echo "| ${run_num} | ${profile_file} | ${report_path} |" >> "${summary_file}"
done

printf '[OK] Summary report: %s\n' "${summary_file}"
cat "${summary_file}"
