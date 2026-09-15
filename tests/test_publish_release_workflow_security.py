#!/usr/bin/env python3
"""
Static/text-based security assertions for the Phase 3 Publish Release
workflow, mirroring tests/test_prepare_release_workflow_security.py's own
approach (regex over the raw YAML text, no third-party YAML dependency --
this repo is stdlib-only Python). scripts/check_workflow_security.py
already covers the generic rules (explicit permissions block, no
write-all, no pull_request_target+checkout, pinned action refs); this file
covers the Phase-3-specific properties from CLAUDE.md's release-automation
section that a generic scan can't express.
"""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PUBLISH = ROOT / ".github" / "workflows" / "publish-release.yml"

publish_text = PUBLISH.read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# workflow_dispatch only, no other trigger.
# ---------------------------------------------------------------------------

on_block_match = re.search(r"^on:\n((?:  .+\n)+)", publish_text, re.MULTILINE)
assert on_block_match, "publish-release.yml missing a parseable `on:` block"
on_block = on_block_match.group(1)
assert "workflow_dispatch" in on_block
for forbidden_trigger in ("push:", "pull_request:", "pull_request_target:", "schedule:", "workflow_run:", "release:"):
    assert forbidden_trigger not in on_block, f"publish-release.yml unexpectedly triggers on {forbidden_trigger}"

print("PASS: publish-release.yml triggers on workflow_dispatch only")


# ---------------------------------------------------------------------------
# Required version + expected_sha inputs, both validated against a strict
# regex before any use.
# ---------------------------------------------------------------------------

assert "version:" in on_block
assert "expected_sha:" in on_block
assert on_block.count("required: true") >= 2, "both version and expected_sha must be required inputs"

assert re.search(r"\^\[0-9\]\+\\\.\[0-9\]\+\\\.\[0-9\]\+\$", publish_text), (
    "publish-release.yml must validate the version input against a bare-semver regex"
)
assert re.search(r"\^\[0-9a-fA-F\]\{40\}\$", publish_text), (
    "publish-release.yml must validate the expected_sha input against a strict 40-hex regex"
)

print("PASS: publish-release.yml requires `version` and `expected_sha` inputs and validates both strictly")


# ---------------------------------------------------------------------------
# Explicit top-level permissions block, no write-all, minimal (read-only)
# default GITHUB_TOKEN -- the privileged App token is a completely
# separate credential.
# ---------------------------------------------------------------------------

perm_match = re.search(r"^permissions:\n((?:  .+\n)+)", publish_text, re.MULTILINE)
assert perm_match, "publish-release.yml missing a top-level permissions block"
permissions_block = perm_match.group(1)
assert "write" not in permissions_block, f"publish-release.yml default token grants write: {permissions_block!r}"
assert "contents: read" in permissions_block
assert "write-all" not in publish_text

print("PASS: publish-release.yml default GITHUB_TOKEN permissions are read-only, no write-all")


# ---------------------------------------------------------------------------
# No pull_request_target trigger, no secrets: inherit, no untrusted
# checkout while a privileged token is available (this workflow only ever
# checks out `main`, once, before the App token is even minted).
# ---------------------------------------------------------------------------

TRIGGER_KEY_RE = re.compile(r"^\s*pull_request_target:\s*$", re.MULTILINE)
assert not TRIGGER_KEY_RE.search(publish_text)
assert "secrets: inherit" not in publish_text

checkout_positions = [m.start() for m in re.finditer(r"uses:\s*actions/checkout@", publish_text)]
assert len(checkout_positions) == 1, f"expected exactly one checkout step, found {len(checkout_positions)}"

mint_token_pos = publish_text.index("create-github-app-token")
assert checkout_positions[0] < mint_token_pos, "checkout must happen before the App token is minted"
after_mint = publish_text[mint_token_pos:]
assert "actions/checkout" not in after_mint, "a checkout occurs after the App token is minted"

print("PASS: publish-release.yml has no pull_request_target trigger, no secrets: inherit, and checks out main only once, before the App token is minted")


# ---------------------------------------------------------------------------
# Candidate validation runs, under the default token, before the App token
# is minted -- and the App token is minted only after that preflight
# succeeds.
# ---------------------------------------------------------------------------

candidate_step_pos = publish_text.index("check_release.py candidate")
assert candidate_step_pos < mint_token_pos, "candidate validation must run before the App token is minted"

print("PASS: `candidate` mode preflight runs, under the default read-only token, before the App token is minted")


# ---------------------------------------------------------------------------
# The App token requests Contents: write ONLY -- no Pull requests: write
# (Publish never opens/updates a PR, unlike Prepare Release).
# ---------------------------------------------------------------------------

app_token_step_match = re.search(
    r"uses:\s*actions/create-github-app-token@[^\n]+\n((?:\s{2,}.+\n)+)", publish_text
)
assert app_token_step_match, "publish-release.yml missing a parseable create-github-app-token step"
app_token_step = app_token_step_match.group(1)
assert "permission-contents: write" in app_token_step
assert "permission-pull-requests" not in app_token_step, (
    "Publish Release's App token must not request Pull requests: write"
)

print("PASS: the App token requests Contents: write only, no Pull requests: write")


# ---------------------------------------------------------------------------
# inputs.version / inputs.expected_sha are each interpolated directly into
# a shell command exactly once -- the env: assignment inside the
# validation step -- never used raw anywhere else (e.g. inside a `run:`
# body via string interpolation).
# ---------------------------------------------------------------------------

for input_name, env_name in (("version", "RAW_VERSION"), ("expected_sha", "RAW_SHA")):
    raw_occurrences = [
        line for line in publish_text.splitlines() if f"${{{{ inputs.{input_name} }}}}" in line
    ]
    assert len(raw_occurrences) == 1, (
        f"expected exactly one direct reference to inputs.{input_name} (the env: assignment), "
        f"found {len(raw_occurrences)}: {raw_occurrences}"
    )
    assert f"{env_name}:" in raw_occurrences[0], (
        f"the sole inputs.{input_name} reference must be the {env_name} env: assignment, not inline shell use"
    )

print("PASS: inputs.version / inputs.expected_sha are each only ever assigned to an env var, never interpolated directly into a shell command")


# ---------------------------------------------------------------------------
# The App token itself is never referenced inside a `run:` shell block as a
# literal GitHub-expression interpolation -- only via env: (GH_TOKEN) or an
# action's `with:` input, matching this repo's existing gh-CLI convention
# (GH_TOKEN: ${{ github.token }} used the same way elsewhere).
# ---------------------------------------------------------------------------

app_token_ref = "steps.app-token.outputs.token"
assert app_token_ref in publish_text
token_lines = [line for line in publish_text.splitlines() if app_token_ref in line]
TOKEN_LINE_RE = re.compile(
    r"^\s*(token:|GH_TOKEN:)\s*\$\{\{\s*steps\.app-token\.outputs\.token\s*\}\}\s*$"
)
assert all(TOKEN_LINE_RE.match(line) for line in token_lines), (
    f"the App token output must only ever appear as a `token:`/`GH_TOKEN:` key-value line, "
    f"never inline inside a shell command body: {token_lines}"
)

print("PASS: the App token is only ever passed via a `token:`/`GH_TOKEN:` key-value line, never inline in a shell command body")


# ---------------------------------------------------------------------------
# The privileged publish step runs `check_release.py publish` (not
# `candidate`/`prepare`), and it is the only step using the App token's
# GH_TOKEN -- the candidate preflight step above uses github.token instead.
# ---------------------------------------------------------------------------

publish_step_pos = publish_text.index("check_release.py publish")
assert publish_step_pos > mint_token_pos, "the privileged publish step must run after the App token is minted"

candidate_step_env = publish_text[publish_text.index("Run Phase 1 checker in candidate mode"):candidate_step_pos]
assert "github.token" in candidate_step_env, "the candidate preflight step must use the default github.token, not the App token"

print("PASS: `check_release.py publish` runs after token mint using the App token; the earlier candidate preflight uses the default github.token")

print("PASS: publish-release workflow security assertions")
