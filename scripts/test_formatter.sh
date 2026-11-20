#!/usr/bin/env bash

set -u

STREAMNZB_REPO="https://github.com/Gaisberg/streamnzb.git"
STREAMNZB_REF="${STREAMNZB_REF:-63a37b614db9246cd65aec08e42b93a91e9068d0}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKOUT="${ROOT}/.streamnzb-compat"
HARNESS="${ROOT}/tests/streamnzb_compat"

TEST_SOURCE="${HARNESS}/upstream/dracula_formatter_compat_test.go"
TEST_TARGET="${CHECKOUT}/pkg/server/stremio/dracula_formatter_compat_test.go"
# The Jellyfin formatter has its own regression (stremio-package label tests plus
# a pkg/server/jellyfin PlaybackInfo test); both files are injected only for it.
JELLYFIN_TEST_SOURCE="${HARNESS}/upstream/dracula_jellyfin_compat_test.go"
JELLYFIN_TEST_TARGET="${CHECKOUT}/pkg/server/stremio/dracula_jellyfin_compat_test.go"
JELLYFIN_MEDIA_TEST_SOURCE="${HARNESS}/upstream/dracula_jellyfin_media_source_test.go"
JELLYFIN_MEDIA_TEST_TARGET="${CHECKOUT}/pkg/server/jellyfin/dracula_jellyfin_media_source_test.go"
JELLYFIN=0
# Set only after this run creates its own scratch file; the cleanup trap removes
# nothing else, so a JELLYFIN_ENTRIES inherited from the caller is never touched.
JELLYFIN_ENTRIES=""
JELLYFIN_STAGE_LOG=""

MODE="candidate"
FORMATTER="${ROOT}/tests/streamnzb_compat/formatter.source.json"
CASES="${HARNESS}/fixtures/formatter.json"

case "${1:-}" in
    "")
        ;;
    --production)
        MODE="production"
        FORMATTER="${ROOT}/formatter.txt"
        ;;
    --debug)
        MODE="debug-candidate"
        FORMATTER="${HARNESS}/formatter-debug.source.json"
        CASES="${HARNESS}/fixtures/formatter-debug.json"
        ;;
    --debug-production)
        MODE="debug-production"
        FORMATTER="${ROOT}/formatter-debug.txt"
        CASES="${HARNESS}/fixtures/formatter-debug.json"
        ;;
    --jellyfin)
        MODE="jellyfin-candidate"
        FORMATTER="${HARNESS}/formatter-jellyfin.source.json"
        CASES="${HARNESS}/fixtures/formatter-jellyfin.json"
        JELLYFIN=1
        ;;
    --jellyfin-production)
        MODE="jellyfin-production"
        FORMATTER="${ROOT}/formatter-jellyfin.txt"
        CASES="${HARNESS}/fixtures/formatter-jellyfin.json"
        JELLYFIN=1
        ;;
    *)
        echo \
            "Usage: $0 [--production|--debug|--debug-production|--jellyfin|--jellyfin-production]" \
            >&2
        exit 2
        ;;
esac

echo "==> DraCuLa formatter simulation"
echo "    mode: ${MODE}"
echo "    ref:  ${STREAMNZB_REF}"
echo "    file: ${FORMATTER}"

for required in \
    "${FORMATTER}" \
    "${TEST_SOURCE}" \
    "${JELLYFIN_TEST_SOURCE}" \
    "${JELLYFIN_MEDIA_TEST_SOURCE}" \
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
rm -f "${TEST_TARGET}" "${JELLYFIN_TEST_TARGET}" "${JELLYFIN_MEDIA_TEST_TARGET}"

echo "==> Fetching pinned StreamNZB revision"

# shellcheck source=scripts/streamnzb_ref.sh
source "${ROOT}/scripts/streamnzb_ref.sh" || exit 1

EXPECTED_REF="$(
    resolve_streamnzb_ref "${CHECKOUT}" "${STREAMNZB_REF}"
)" || exit 1

git -C "${CHECKOUT}" \
    checkout \
    --quiet \
    --detach \
    "${EXPECTED_REF}"

CHECKOUT_RC=$?

if [[ "${CHECKOUT_RC}" -ne 0 ]]; then
    echo \
        "ERROR: StreamNZB checkout failed" \
        >&2
    exit "${CHECKOUT_RC}"
fi

verify_streamnzb_checkout \
    "${CHECKOUT}" \
    "${STREAMNZB_REF}" \
    "${EXPECTED_REF}" \
    || exit 1

echo "    commit: ${EXPECTED_REF}"

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
    rm -f "${TEST_TARGET}" "${JELLYFIN_TEST_TARGET}" "${JELLYFIN_MEDIA_TEST_TARGET}"
    if [[ -n "${JELLYFIN_ENTRIES}" ]]; then
        rm -f "${JELLYFIN_ENTRIES}"
    fi
    if [[ -n "${JELLYFIN_STAGE_LOG}" ]]; then
        rm -f "${JELLYFIN_STAGE_LOG}"
    fi
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

if [[ "${JELLYFIN}" -eq 1 ]]; then
    cp "${JELLYFIN_TEST_SOURCE}" "${JELLYFIN_TEST_TARGET}" \
        && cp "${JELLYFIN_MEDIA_TEST_SOURCE}" "${JELLYFIN_MEDIA_TEST_TARGET}"
    JELLYFIN_COPY_RC=$?

    if [[ "${JELLYFIN_COPY_RC}" -ne 0 ]]; then
        echo \
            "ERROR: could not inject Jellyfin formatter compatibility tests" \
            >&2
        exit "${JELLYFIN_COPY_RC}"
    fi
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

if [[ "${JELLYFIN}" -eq 1 ]]; then
    # Stage 1 renders real ranked candidates through StreamNZB's playlist
    # aggregation and hands the labels to stage 2, which sends them through the
    # real Jellyfin PlaybackInfo handler. The description-policy test proves the
    # empty-description allowance does not extend to the other formatters.
    JELLYFIN_ENTRIES="$(mktemp "${TMPDIR:-/tmp}/dracula-jellyfin-entries.XXXXXX")" || {
        JELLYFIN_ENTRIES=""
        echo "ERROR: could not create the Jellyfin scratch file" >&2
        exit 1
    }

    #
    # `go test -run` exits 0 when the pattern matches nothing, so each stage must
    # show its tests passing and stage one must produce labels. Runs that share one
    # StreamNZB checkout must not overlap: every run injects and removes these files.
    JELLYFIN_STAGE_LOG="$(mktemp "${TMPDIR:-/tmp}/dracula-jellyfin-stage.XXXXXX")" || {
        JELLYFIN_STAGE_LOG=""
        echo "ERROR: could not create the Jellyfin stage log" >&2
        exit 1
    }

    DRACULA_FORMATTER_PATH="${FORMATTER}" \
    DRACULA_FORMATTER_CASES_PATH="${CASES}" \
    DRACULA_JELLYFIN_ENTRIES="${JELLYFIN_ENTRIES}" \
    go test \
        -v \
        -count=1 \
        ./pkg/server/stremio \
        -run '^(TestDraculaJellyfinFormatter|TestDraculaFormatterDescriptionPolicy)$' \
        2>&1 | tee "${JELLYFIN_STAGE_LOG}"

    TEST_RC="${PIPESTATUS[0]}"

    if [[ "${TEST_RC}" -eq 0 ]] && {
        ! grep -q '^--- PASS: TestDraculaJellyfinFormatter ' "${JELLYFIN_STAGE_LOG}" \
            || ! grep -q '^--- PASS: TestDraculaFormatterDescriptionPolicy ' "${JELLYFIN_STAGE_LOG}" \
            || [[ ! -s "${JELLYFIN_ENTRIES}" ]]
    }; then
        echo "ERROR: the Jellyfin label stage did not run its tests or wrote no labels" >&2
        TEST_RC=1
    fi

    if [[ "${TEST_RC}" -eq 0 ]]; then
        echo "==> Sending the rendered labels through StreamNZB's Jellyfin PlaybackInfo handler"

        DRACULA_JELLYFIN_ENTRIES="${JELLYFIN_ENTRIES}" \
        go test \
            -v \
            -count=1 \
            ./pkg/server/jellyfin \
            -run '^TestDraculaJellyfinMediaSourceNames$' \
            2>&1 | tee "${JELLYFIN_STAGE_LOG}"

        TEST_RC="${PIPESTATUS[0]}"

        if [[ "${TEST_RC}" -eq 0 ]] \
            && ! grep -q '^--- PASS: TestDraculaJellyfinMediaSourceNames ' "${JELLYFIN_STAGE_LOG}"; then
            echo "ERROR: the Jellyfin PlaybackInfo stage did not run its test" >&2
            TEST_RC=1
        fi
    fi
else
    DRACULA_FORMATTER_PATH="${FORMATTER}" \
    DRACULA_FORMATTER_CASES_PATH="${CASES}" \
    go test \
        -v \
        -count=1 \
        ./pkg/server/stremio \
        -run '^TestDraculaFormatterFixtures$'

    TEST_RC=$?
fi

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
