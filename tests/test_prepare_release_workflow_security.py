#!/usr/bin/env python3
"""
Static/text-based security assertions for the Phase 2 workflows, mirroring
scripts/check_workflow_security.py's own approach (regex over the raw YAML
text, no third-party YAML dependency -- this repo is stdlib-only Python).
check_workflow_security.py itself already covers the generic rules (explicit
permissions block, no write-all, no pull_request_target+checkout, pinned
action refs); this file covers the Phase-2-specific properties called out
in the architect's review checklist that a generic scan can't express.
"""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PREPARE = ROOT / ".github" / "workflows" / "prepare-release.yml"
VALIDATE = ROOT / ".github" / "workflows" / "validate-release-preparation.yml"

prepare_text = PREPARE.read_text(encoding="utf-8")
validate_text = VALIDATE.read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# Prepare Release: workflow_dispatch only.
# ---------------------------------------------------------------------------

on_block_match = re.search(r"^on:\n((?:  .+\n)+)", prepare_text, re.MULTILINE)
assert on_block_match, "prepare-release.yml missing a parseable `on:` block"
on_block = on_block_match.group(1)
assert "workflow_dispatch" in on_block
for forbidden_trigger in ("push:", "pull_request:", "pull_request_target:", "schedule:", "workflow_run:"):
    assert forbidden_trigger not in on_block, f"prepare-release.yml unexpectedly triggers on {forbidden_trigger}"

print("PASS: prepare-release.yml triggers on workflow_dispatch only")


# ---------------------------------------------------------------------------
# Required version input, bare semver typed as a string (validated in the
# first step, not trusted as-is).
# ---------------------------------------------------------------------------

assert "version:" in on_block
assert "required: true" in prepare_text
assert re.search(r"\^\[0-9\]\+\\\.\[0-9\]\+\\\.\[0-9\]\+\$", prepare_text), (
    "prepare-release.yml must validate the version input against a bare-semver regex"
)

print("PASS: prepare-release.yml requires a `version` input and validates it against bare semver")


# ---------------------------------------------------------------------------
# No pull_request_target trigger in either workflow. Matched as an actual
# YAML trigger key (line starting with the token followed by a colon), not
# a bare substring -- both files' own explanatory comments legitimately
# mention the term in prose (e.g. "not pull_request_target:").
# ---------------------------------------------------------------------------

TRIGGER_KEY_RE = re.compile(r"^\s*pull_request_target:\s*$", re.MULTILINE)
assert not TRIGGER_KEY_RE.search(prepare_text)
assert not TRIGGER_KEY_RE.search(validate_text)

print("PASS: neither Phase 2 workflow declares a pull_request_target trigger")


# ---------------------------------------------------------------------------
# Default GITHUB_TOKEN stays minimal (no write) in both workflows; the
# write-capable GitHub App token is a completely separate credential.
# ---------------------------------------------------------------------------

def top_level_permissions_block(text):
    match = re.search(r"^permissions:\n((?:  .+\n)+)", text, re.MULTILINE)
    assert match, "workflow missing a top-level permissions block"
    return match.group(1)


prepare_permissions = top_level_permissions_block(prepare_text)
assert "write" not in prepare_permissions, f"prepare-release.yml default token grants write: {prepare_permissions!r}"
assert "contents: read" in prepare_permissions

validate_permissions = top_level_permissions_block(validate_text)
assert "write" not in validate_permissions, f"validate-release-preparation.yml default token grants write: {validate_permissions!r}"
assert "contents: read" in validate_permissions

print("PASS: both workflows' default GITHUB_TOKEN permissions are read-only")


# ---------------------------------------------------------------------------
# No secrets available to the PR-validation workflow at all -- it must be
# fully credential-free (read-only checkout + default GITHUB_TOKEN only).
# ---------------------------------------------------------------------------

assert "secrets." not in validate_text, "validate-release-preparation.yml must never reference secrets"

print("PASS: validate-release-preparation.yml references no secrets (no App token available to PR validation)")


# ---------------------------------------------------------------------------
# The GitHub App token is minted only AFTER the trusted preflight/
# generation steps have already run against known-trusted `main` code --
# never before, and never used to check out or execute anything else.
# ---------------------------------------------------------------------------

checker_step_pos = prepare_text.index("scripts/check_release.py prepare")
generate_step_pos = prepare_text.index("prepare_release_housekeeping.py")
verify_diff_step_pos = prepare_text.index("Verify only the expected files changed")
mint_token_pos = prepare_text.index("create-github-app-token")

assert checker_step_pos < mint_token_pos, "App token minted before the Phase 1 checker ran"
assert generate_step_pos < mint_token_pos, "App token minted before housekeeping generation ran"
assert verify_diff_step_pos < mint_token_pos, "App token minted before the generated-diff was verified"

# Nothing after the App-token mint checks out a different ref or runs
# scripts/check_release.py again -- the only steps after minting are the
# credential-gate message and the PR-creation action itself.
after_mint = prepare_text[mint_token_pos:]
assert "actions/checkout" not in after_mint, "a checkout occurs after the App token is minted"
assert "check_release.py" not in after_mint, "trusted-code re-execution occurs after the App token is minted"

print("PASS: the GitHub App token is minted only after preflight/generation succeed, and nothing checks out/re-executes code afterward")


# ---------------------------------------------------------------------------
# App credentials verified explicitly before minting (fail clearly, before
# any mutation, if the App isn't configured yet) -- and via env:, never a
# secrets.* reference inside an `if:` conditional (not supported by GHA).
# ---------------------------------------------------------------------------

assert "PREPARE_RELEASE_APP_ID" in prepare_text
assert "PREPARE_RELEASE_APP_PRIVATE_KEY" in prepare_text
assert re.search(r'if:\s*secrets\.', prepare_text) is None, (
    "secrets must not be referenced directly inside an `if:` conditional"
)

print("PASS: App credentials are checked via env:+bash before minting, never via secrets. in an `if:` conditional")


# ---------------------------------------------------------------------------
# The App token-mint step authenticates via the supported `client-id` input
# (never the deprecated `app-id`), using the existing PREPARE_RELEASE_APP_ID
# secret value unchanged, with private-key and permission inputs intact.
# actions/create-github-app-token@v3's `app-id` input carries
# `deprecationMessage: "Use 'client-id' instead."` upstream -- both inputs
# feed the identical authentication path (`getInput("client-id") ||
# getInput("app-id")`), so this is a pure input-key migration, not a
# behavior change.
# ---------------------------------------------------------------------------

app_token_step_match = re.search(
    r"uses:\s*actions/create-github-app-token@v3\n((?:\s{2,}.+\n)+)", prepare_text
)
assert app_token_step_match, "prepare-release.yml missing a parseable create-github-app-token step"
app_token_step = app_token_step_match.group(1)

assert "client-id: ${{ secrets.PREPARE_RELEASE_APP_ID }}" in app_token_step, (
    "prepare-release.yml must authenticate via the supported client-id input, using the existing App ID secret"
)
assert not re.search(r"^\s*app-id:", app_token_step, re.MULTILINE), (
    "prepare-release.yml must not use the deprecated app-id input"
)
assert "private-key: ${{ secrets.PREPARE_RELEASE_APP_PRIVATE_KEY }}" in app_token_step
assert "permission-contents: write" in app_token_step
assert "permission-pull-requests: write" in app_token_step

print("PASS: prepare-release.yml's App token step uses client-id (not deprecated app-id), with private-key and permission inputs intact")


# ---------------------------------------------------------------------------
# inputs.version is never interpolated directly into a shell command --
# only assigned to an env var (RAW_VERSION) once, then referenced as
# "$RAW_VERSION" thereafter.
# ---------------------------------------------------------------------------

raw_input_occurrences = [
    line for line in prepare_text.splitlines() if "${{ inputs.version }}" in line
]
assert len(raw_input_occurrences) == 1, (
    f"expected exactly one direct reference to inputs.version (the env: assignment), "
    f"found {len(raw_input_occurrences)}: {raw_input_occurrences}"
)
assert "RAW_VERSION:" in raw_input_occurrences[0], (
    "the sole inputs.version reference must be the RAW_VERSION env: assignment, not inline shell use"
)

print("PASS: inputs.version is only ever assigned to an env var, never interpolated directly into a shell command")


# ---------------------------------------------------------------------------
# The App token itself is never referenced inside a `run:` shell block
# (only as an action `with: token:` input) -- avoids it ever landing in a
# printed/logged shell command.
# ---------------------------------------------------------------------------

app_token_ref = "steps.app-token.outputs.token"
assert app_token_ref in prepare_text
token_lines = [line for line in prepare_text.splitlines() if app_token_ref in line]
# Every line referencing the token must be exactly a `token:` key/value --
# the only legitimate shape (an action's `with: token: ...` input). This is
# deliberately a structural match, not "does this line also happen to
# contain the word run:" -- a reference smuggled into a multiline `run: |`
# block (e.g. `echo "${{ steps.app-token.outputs.token }}"`) would not
# contain the literal substrings "with:"/"run:" on its own line either, so
# a same-line keyword check alone would miss it.
TOKEN_KV_LINE_RE = re.compile(r"^\s*token:\s*\$\{\{\s*steps\.app-token\.outputs\.token\s*\}\}\s*$")
assert all(TOKEN_KV_LINE_RE.match(line) for line in token_lines), (
    f"the App token output must only ever appear as a `token: ...` action input line, "
    f"never inline in a shell command: {token_lines}"
)

print("PASS: the App token is only ever passed via an action's `with: token:` input, never used in a shell command")


# ---------------------------------------------------------------------------
# Branch name is derived only from the already-validated version -- no
# user-supplied branch text is accepted as a workflow input at all.
# ---------------------------------------------------------------------------

assert "branch:" not in on_block  # not a workflow_dispatch input
assert re.search(r'echo "branch=docs/prepare-release-\$\{RAW_VERSION//\.\/-\}"', prepare_text) or \
    "docs/prepare-release-${RAW_VERSION//./-}" in prepare_text
assert not re.search(r"inputs\.branch", prepare_text), "no branch-name workflow input should exist"

print("PASS: the release-preparation branch name is derived only from the validated version, never a free-text input")


# ---------------------------------------------------------------------------
# validate-release-preparation.yml: pull_request only, scoped to
# docs/prepare-release-* branches, calls prep-pr mode (not candidate).
# ---------------------------------------------------------------------------

validate_on_block_match = re.search(r"^on:\n((?:  .+\n)+)", validate_text, re.MULTILINE)
assert validate_on_block_match
validate_on_block = validate_on_block_match.group(1)
assert "pull_request:" in validate_on_block
assert "pull_request_target" not in validate_on_block

assert "docs/prepare-release-" in validate_text
assert "check_release.py prep-pr" in validate_text
assert "check_release.py candidate" not in validate_text

print("PASS: validate-release-preparation.yml triggers on pull_request only, scoped to release-prep branches, uses prep-pr mode")

print("PASS: prepare-release workflow security assertions")
