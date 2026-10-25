# Shared StreamNZB revision helpers. Source this file; do not execute it.

# Fetch STREAMNZB_REF (commit SHA, lightweight tag or annotated tag) and print
# the commit it peels to. Uses FETCH_HEAD so a stale local tag cannot win.
resolve_streamnzb_ref() {
    local checkout="$1"
    local ref="$2"
    local commit

    if ! git -C "${checkout}" fetch --quiet origin "${ref}"; then
        echo \
            "ERROR: StreamNZB fetch failed for ${ref}" \
            >&2
        return 1
    fi

    if ! commit="$(
        git -C "${checkout}" rev-parse --verify --quiet "FETCH_HEAD^{commit}"
    )"; then
        echo \
            "ERROR: StreamNZB ${ref} does not resolve to a commit" \
            >&2
        return 1
    fi

    if [[ "${ref}" =~ ^[0-9a-f]{40}$ && "${commit}" != "${ref}" ]]; then
        echo \
            "ERROR: expected StreamNZB ${ref}, fetched ${commit}" \
            >&2
        return 1
    fi

    printf '%s\n' "${commit}"
}

# Fail unless the checkout HEAD is exactly the resolved commit.
verify_streamnzb_checkout() {
    local checkout="$1"
    local ref="$2"
    local expected="$3"
    local actual

    if ! actual="$(git -C "${checkout}" rev-parse --verify HEAD)"; then
        echo \
            "ERROR: could not read StreamNZB checkout revision" \
            >&2
        return 1
    fi

    if [[ "${actual}" != "${expected}" ]]; then
        echo \
            "ERROR: expected StreamNZB ${ref} (${expected}), got ${actual}" \
            >&2
        return 1
    fi
}
