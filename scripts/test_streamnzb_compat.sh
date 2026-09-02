#!/usr/bin/env bash

set -u

STREAMNZB_REPO="https://github.com/Gaisberg/streamnzb.git"
STREAMNZB_REF="0429d50b347000325fa40ecf8aeb670335d23260"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKOUT="${ROOT}/.streamnzb-compat"
HARNESS="${ROOT}/tests/streamnzb_compat"

echo "==> StreamNZB compatibility harness"
echo "    ref: ${STREAMNZB_REF}"

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

echo "==> Using StreamNZB ${ACTUAL_REF}"

echo "==> Checking formatter source/published synchronization"

python3 \
    "${ROOT}/scripts/build_formatter.py" \
    --check

SYNC_RC=$?

if [[ "${SYNC_RC}" -ne 0 ]]; then
    echo \
        "ERROR: formatter synchronization check failed" \
        >&2
    exit "${SYNC_RC}"
fi

python3 \
    "${ROOT}/scripts/build_formatter.py" \
    --source "tests/streamnzb_compat/formatter-debug.source.json" \
    --output "formatter-debug.txt" \
    --check

DEBUG_SYNC_RC=$?

if [[ "${DEBUG_SYNC_RC}" -ne 0 ]]; then
    echo \
        "ERROR: debug formatter synchronization check failed" \
        >&2
    exit "${DEBUG_SYNC_RC}"
fi

echo "==> Running rule/profile compatibility tests"

cd "${HARNESS}" || exit 1

go test -v
RULE_RC=$?

if [[ "${RULE_RC}" -ne 0 ]]; then
    echo \
        "ERROR: rule/profile compatibility tests failed" \
        >&2
    exit "${RULE_RC}"
fi

echo
echo "==> Running published formatter regression"

cd "${ROOT}" || exit 1

./scripts/test_formatter.sh --production
FORMATTER_RC=$?

if [[ "${FORMATTER_RC}" -ne 0 ]]; then
    echo \
        "ERROR: formatter compatibility tests failed" \
        >&2
    exit "${FORMATTER_RC}"
fi

echo
echo "==> Running published debug formatter regression"

./scripts/test_formatter.sh --debug-production
DEBUG_FORMATTER_RC=$?

if [[ "${DEBUG_FORMATTER_RC}" -ne 0 ]]; then
    echo \
        "ERROR: debug formatter compatibility tests failed" \
        >&2
    exit "${DEBUG_FORMATTER_RC}"
fi

echo
echo "StreamNZB compatibility tests passed."

exit 0
