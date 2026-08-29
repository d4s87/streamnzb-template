#!/usr/bin/env bash
set -euo pipefail

STREAMNZB_REPO="https://github.com/Gaisberg/streamnzb.git"
STREAMNZB_REF="9b577f7fc226446dc74f7bc2724b102c725eef8a"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKOUT="${ROOT}/.streamnzb-compat"
HARNESS="${ROOT}/tests/streamnzb_compat"

echo "==> StreamNZB compatibility harness"
echo "    ref: ${STREAMNZB_REF}"

if [[ ! -d "${CHECKOUT}/.git" ]]; then
    echo "==> Cloning StreamNZB"
    rm -rf "${CHECKOUT}"
    git clone --quiet "${STREAMNZB_REPO}" "${CHECKOUT}"
fi

echo "==> Fetching pinned StreamNZB revision"
git -C "${CHECKOUT}" fetch --quiet origin "${STREAMNZB_REF}"
git -C "${CHECKOUT}" checkout --quiet --detach "${STREAMNZB_REF}"

ACTUAL_REF="$(git -C "${CHECKOUT}" rev-parse HEAD)"

if [[ "${ACTUAL_REF}" != "${STREAMNZB_REF}" ]]; then
    echo "ERROR: expected StreamNZB ${STREAMNZB_REF}, got ${ACTUAL_REF}" >&2
    exit 1
fi

echo "==> Using StreamNZB ${ACTUAL_REF}"
echo "==> Running compatibility tests"

cd "${HARNESS}"
go test -v

echo
echo "StreamNZB compatibility tests passed."
