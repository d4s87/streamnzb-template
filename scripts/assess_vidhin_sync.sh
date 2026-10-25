#!/usr/bin/env bash

# Advisory Vidhin sync assessment (docs/VIDHIN_SYNC_ASSESSMENT.md).
#
# Compares the working-tree generated Vidhin files against BASE_REF, runs the
# real-engine compatibility suite together with old-vs-new Define Library
# probes, and writes assessment.json and assessment.md into OUT_DIR.
#
# Usage: scripts/assess_vidhin_sync.sh [BASE_REF] [OUT_DIR]
#
# Exits non-zero only when the assessment itself could not be produced. A
# BLOCKED decision is a result, not a script failure.

set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_REF="${1:-HEAD}"
OUT_DIR="${2:-}"

if [[ -z "${OUT_DIR}" ]]; then
    OUT_DIR="$(mktemp -d)" || exit 1
fi

mkdir -p "${OUT_DIR}" || exit 1
OUT_DIR="$(cd "${OUT_DIR}" && pwd)"

echo "==> Vidhin sync assessment (advisory)"
echo "    base: ${BASE_REF}"
echo "    out:  ${OUT_DIR}"

for pair in \
    "generated/vidhin-defines.json:old-baseline.json" \
    "generated/streamnzb-defines.txt:old-library.txt"
do
    if ! git -C "${ROOT}" show "${BASE_REF}:${pair%%:*}" \
        > "${OUT_DIR}/${pair##*:}"; then
        echo \
            "ERROR: could not read ${pair%%:*} at ${BASE_REF}" \
            >&2
        exit 1
    fi
done

ARGS=(
    --old-baseline "${OUT_DIR}/old-baseline.json"
    --old-library "${OUT_DIR}/old-library.txt"
    --new-baseline "${ROOT}/generated/vidhin-defines.json"
    --new-library "${ROOT}/generated/streamnzb-defines.txt"
    --plan "${OUT_DIR}/probe-plan.json"
)

python3 "${ROOT}/scripts/assess_vidhin_sync.py" plan "${ARGS[@]}" || exit 1

rm -f "${OUT_DIR}/probe-results.json"

echo "==> Running real-engine compatibility suite with sync probes"

VIDHIN_PROBE_PLAN="${OUT_DIR}/probe-plan.json" \
VIDHIN_PROBE_RESULTS="${OUT_DIR}/probe-results.json" \
"${ROOT}/scripts/test_streamnzb_compat.sh" \
    > "${OUT_DIR}/compat.log" 2>&1

if [[ $? -eq 0 ]]; then
    COMPAT_RESULT="passed"
else
    COMPAT_RESULT="failed"
fi

echo "    compatibility suite: ${COMPAT_RESULT} (log: ${OUT_DIR}/compat.log)"

python3 "${ROOT}/scripts/assess_vidhin_sync.py" assess "${ARGS[@]}" \
    --compat-result "${COMPAT_RESULT}" \
    --engine-results "${OUT_DIR}/probe-results.json" \
    --json-out "${OUT_DIR}/assessment.json" \
    --markdown-out "${OUT_DIR}/assessment.md" \
    || exit 1
