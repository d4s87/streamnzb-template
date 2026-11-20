#!/usr/bin/env bash

set -u

STREAMNZB_REPO="https://github.com/Gaisberg/streamnzb.git"
STREAMNZB_REF="${STREAMNZB_REF:-63a37b614db9246cd65aec08e42b93a91e9068d0}"

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

echo "==> Fetching StreamNZB revision"

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
    FETCH_HEAD

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

echo "==> Using StreamNZB ${ACTUAL_REF}"

echo "==> Checking formatter source/published synchronization"

# Production, debug and Jellyfin formatters: scripts/build_formatter.py owns the list.
python3 \
    "${ROOT}/scripts/build_formatter.py" \
    --check \
    --all

SYNC_RC=$?

if [[ "${SYNC_RC}" -ne 0 ]]; then
    echo \
        "ERROR: formatter synchronization check failed" \
        >&2
    exit "${SYNC_RC}"
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
echo "==> Running published Jellyfin formatter regression"

./scripts/test_formatter.sh --jellyfin-production
JELLYFIN_FORMATTER_RC=$?

if [[ "${JELLYFIN_FORMATTER_RC}" -ne 0 ]]; then
    echo \
        "ERROR: Jellyfin formatter compatibility tests failed" \
        >&2
    exit "${JELLYFIN_FORMATTER_RC}"
fi

echo
echo "StreamNZB compatibility tests passed."

exit 0
