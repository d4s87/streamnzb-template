#!/usr/bin/env bash

set -u

STREAMNZB_REPO="https://github.com/Gaisberg/streamnzb.git"
STREAMNZB_REF="4c0f7b385e5f7bfb514523b908fa04f153dfbbe2"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKOUT="${ROOT}/.streamnzb-compat"
HARNESS="${ROOT}/tests/streamnzb_compat"

TEST_SOURCE="${HARNESS}/upstream/dracula_formatter_compat_test.go"
TEST_TARGET="${CHECKOUT}/pkg/server/stremio/dracula_formatter_compat_test.go"
CASES="${HARNESS}/fixtures/formatter.json"

MODE="candidate"
FORMATTER="${ROOT}/tests/streamnzb_compat/formatter.source.json"

if [[ "${1:-}" == "--production" ]]; then
    MODE="production"
    FORMATTER="${ROOT}/formatter.txt"
elif [[ "${1:-}" != "" ]]; then
    echo "Usage: $0 [--production]" >&2
    exit 2
fi

echo "==> DraCuLa formatter simulation"
echo "    mode: ${MODE}"
echo "    ref:  ${STREAMNZB_REF}"
echo "    file: ${FORMATTER}"

for required in \
    "${FORMATTER}" \
    "${TEST_SOURCE}" \
    "${CASES}"
do
    if [[ ! -f "${required}" ]]; then
        echo \
            "ERROR: required file not found: ${required}" \
            >&2
        exit 1
    fi
done

if [[ ! -d "${CHECKOUT}/.git" ]]; then
    echo "==> Cloning StreamNZB"

    rm -rf "${CHECKOUT}"

    git clone \
        --quiet \
        "${STREAMNZB_REPO}" \
        "${CHECKOUT}"

    CLONE_RC=$?

    if [[ "${CLONE_RC}" -ne 0 ]]; then
        echo \
            "ERROR: StreamNZB clone failed" \
            >&2
        exit "${CLONE_RC}"
    fi
fi

TEST_TARGET_DIR="$(
    dirname "${TEST_TARGET}"
)"

mkdir -p "${TEST_TARGET_DIR}"
MKDIR_RC=$?

if [[ "${MKDIR_RC}" -ne 0 ]]; then
    echo \
        "ERROR: could not create formatter test target directory" \
        >&2
    exit "${MKDIR_RC}"
fi

# Remove an injected test left by an interrupted previous run before switching
# the pinned checkout.
rm -f "${TEST_TARGET}"

echo "==> Fetching pinned StreamNZB revision"

git -C "${CHECKOUT}" \
    fetch \
    --quiet \
    origin \
    "${STREAMNZB_REF}"

FETCH_RC=$?

if [[ "${FETCH_RC}" -ne 0 ]]; then
    echo \
        "ERROR: StreamNZB fetch failed" \
        >&2
    exit "${FETCH_RC}"
fi

git -C "${CHECKOUT}" \
    checkout \
    --quiet \
    --detach \
    "${STREAMNZB_REF}"

CHECKOUT_RC=$?

if [[ "${CHECKOUT_RC}" -ne 0 ]]; then
    echo \
        "ERROR: StreamNZB checkout failed" \
        >&2
    exit "${CHECKOUT_RC}"
fi

ACTUAL_REF="$(
    git -C "${CHECKOUT}" rev-parse HEAD
)"

REF_RC=$?

if [[ "${REF_RC}" -ne 0 ]]; then
    echo \
        "ERROR: could not read StreamNZB checkout revision" \
        >&2
    exit "${REF_RC}"
fi

if [[ "${ACTUAL_REF}" != "${STREAMNZB_REF}" ]]; then
    echo \
        "ERROR: expected StreamNZB ${STREAMNZB_REF}, got ${ACTUAL_REF}" \
        >&2
    exit 1
fi

# StreamNZB's web package embeds pkg/server/web/static. The pinned source
# revision does not contain the generated frontend assets, but Go still
# requires at least one matching file before it can compile the stremio
# package and run the formatter tests.
#
# The formatter harness does not exercise the web frontend, so provide a
# disposable placeholder only inside the compatibility checkout.
STATIC_DIR="${CHECKOUT}/pkg/server/web/static"
STATIC_PLACEHOLDER="${STATIC_DIR}/.dracula-formatter-placeholder"

if [[ ! -d "${STATIC_DIR}" ]]; then
    echo "==> Preparing StreamNZB embedded-static prerequisite"

    mkdir -p "${STATIC_DIR}"
    STATIC_MKDIR_RC=$?

    if [[ "${STATIC_MKDIR_RC}" -ne 0 ]]; then
        echo \
            "ERROR: could not create StreamNZB static directory" \
            >&2
        exit "${STATIC_MKDIR_RC}"
    fi
fi

if ! find "${STATIC_DIR}" -type f -print -quit | grep -q .; then
    printf '%s\n' \
        'formatter compatibility placeholder' \
        > "${STATIC_PLACEHOLDER}"

    STATIC_WRITE_RC=$?

    if [[ "${STATIC_WRITE_RC}" -ne 0 ]]; then
        echo \
            "ERROR: could not create StreamNZB static placeholder" \
            >&2
        exit "${STATIC_WRITE_RC}"
    fi
fi

cleanup_formatter_test() {
    rm -f "${TEST_TARGET}"
}

trap cleanup_formatter_test EXIT INT TERM

cp \
    "${TEST_SOURCE}" \
    "${TEST_TARGET}"

COPY_RC=$?

if [[ "${COPY_RC}" -ne 0 ]]; then
    echo \
        "ERROR: could not inject formatter compatibility test" \
        >&2
    exit "${COPY_RC}"
fi

echo "==> Formatter input fingerprints"

python3 - "${FORMATTER}" "${CASES}" <<'PYHASH'
from pathlib import Path
import hashlib
import sys

for raw in sys.argv[1:]:
    path = Path(raw)

    if not path.is_file():
        print(f"ERROR: fingerprint input missing: {path}")
        sys.exit(1)

    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    print(f"    {path}: {digest}")
PYHASH

HASH_RC=$?

if [ "$HASH_RC" -ne 0 ]; then
    echo "ERROR: failed to fingerprint formatter inputs"
    exit "$HASH_RC"
fi

echo "==> Rendering formatter through real StreamNZB engine"

cd "${CHECKOUT}" || exit 1

DRACULA_FORMATTER_PATH="${FORMATTER}" \
DRACULA_FORMATTER_CASES_PATH="${CASES}" \
go test \
    -v \
    -count=1 \
    ./pkg/server/stremio \
    -run '^TestDraculaFormatterFixtures$'

TEST_RC=$?

cleanup_formatter_test
trap - EXIT INT TERM

echo

if [[ "${TEST_RC}" -ne 0 ]]; then
    echo \
        "ERROR: ${MODE} formatter simulation failed"
    exit "${TEST_RC}"
fi

echo \
    "PASS: ${MODE} formatter rendered successfully through StreamNZB."

exit 0
