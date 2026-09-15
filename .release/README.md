# `.release/`

Per-release artifacts produced by the (future, not-yet-implemented) release
housekeeping generator, `scripts/prepare_release_housekeeping.py`. Never
hand-edit the generated fields; the curated release-note prose is the one
part meant for human editing (see below).

This directory is empty as of Phase 1 -- see CLAUDE.md's release-automation
section for the full design. It does **not** contain a `6.0.1.md`/`6.0.1.json`
retroactively: 6.0.1 predates this system, and `check_release.py
verify-published` accepts that one release as a documented legacy exception
via `--allow-missing-release-note` (see `tests/test_check_release_6_0_1_dry_run.py`).
Every release from the one after 6.0.1 onward is expected to carry both
files here.

## `<version>.json` -- preparation provenance

Machine-written at generation time, never hand-edited. Minimal fields:

```json
{
  "schema_version": 1,
  "version": "6.0.2",
  "previous_version": "6.0.1",
  "prepared_from_sha": "<40-char sha main pointed at when this was generated>",
  "prepared_at_date": "YYYY-MM-DD",
  "compatibility": {
    "streamnzb_version": "6.1.0",
    "streamnzb_sha": "<40-char sha>",
    "jhin_version": "0.7.1"
  },
  "expected_counts": {
    "samsung": 151, "neutral": 150, "core": 129,
    "presentation": 21, "device": 1, "defines": 62
  }
}
```

`prepared_from_sha` is the stale-main freshness anchor: at generation time
it equals `main`. While the release-preparation PR is open, a future CI
step re-checks this file's `prepared_from_sha` against the *live* `main`
SHA and must require exact **equality**, not ancestry -- if `main` has
advanced, the PR is stale and the housekeeping must be regenerated from
current `main`, not merged/rebased as-is. See
`check_release.check_preparation_freshness()`. Deliberately does not store
a "candidate SHA" -- the release-prep PR's own merge commit isn't known
until it merges; `check_release.check_preparation_provenance()` verifies
that relationship after the fact, in `candidate` mode.

## `<version>.md` -- authoritative public release note

Generated as a **draft seed** from the matching `CHANGELOG.md` dated
section (verbatim, not summarized -- see `generate_release_note()`'s
docstring for why: mechanical prose condensation without an LLM is
unreliable, and an LLM is explicitly out of scope for this generator). It
carries an HTML-comment TODO marker at the top. The release-preparation PR
is exactly where a human is expected to edit this file into public
release-note tone before merging.

This file, once merged, is what a future Publish Release workflow reads
verbatim as the GitHub release body -- never Release Drafter's
continuously-regenerated draft body (see CLAUDE.md for why Release Drafter
is informational-only and not the publication source of truth).
