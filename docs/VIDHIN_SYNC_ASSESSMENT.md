# Vidhin sync assessment

Every Vidhin synchronization PR carries an advisory assessment with a machine-readable decision: **SAFE**, **REVIEW_REQUIRED** or **BLOCKED**.

The assessment is advisory. It never merges, approves or labels a PR, and it does not decide release impact. Branch protection, human review and human release-impact labeling stay exactly as they were.

## What is compared

The assessment compares the accepted state on `main` (old) with the proposed sync output (new):

- `generated/vidhin-defines.json`: the Vidhin baseline, with upstream records, tokens and generated metadata.
- `generated/streamnzb-defines.txt`: the Define Library users import. This is the behavioral ground truth.
- `scripts/vidhin_mapping.json`: the expected Define set and metadata.
- `profile.txt` and `profile-neutral.txt`: the published Samsung and neutral profiles. They are decoded to find which production rules consume each Define.

Implementation: `scripts/assess_vidhin_sync.py` (the semantic diff and decision), `scripts/assess_vidhin_sync.sh` (the end-to-end run) and `tests/streamnzb_compat/vidhin_sync_probe_test.go` (real-engine probes).

## Decisions

A decision is the most severe decision among its reasons. A change can be SAFE only when **every** reason is on the SAFE allowlist and the evidence is complete.

### SAFE (allowlisted, behavior-preserving)

| Code | Meaning |
|---|---|
| `no_semantic_change` | Nothing changed. |
| `metadata_only` | Only `generated_at_utc` changed. |
| `upstream_pattern_text_only` | Upstream regex text changed, but the extracted tokens, record semantics, metadata and rendered Define are byte-identical. |
| `upstream_records_regrouped` | Upstream split or merged records within the same sources. For token-union modes (`standard`, `generated_dynamic_hdr_groups`), the token union and rendered Define are byte-identical. |

Passing CI or touching only generated files never makes a change SAFE on its own.

### REVIEW_REQUIRED (valid, but intentionally changes behavior)

| Code | Meaning |
|---|---|
| `tier_group_added` / `tier_group_removed` | A release group entered or left a Movie, Show or Anime tier Define. |
| `tier_movement` | A group moved between tiers of one family. |
| `tier_precedence_changed` | WEB tier precedence changed a tier's effective tokens, for example a group promoted from T2 to T1. |
| `trusted_group_added` / `trusted_group_removed` | `Trusted Release Groups` membership changed for a content kind. |
| `classification_group_added` / `classification_group_removed` | A non-tier classification (LQ, Bad Dual, Atmos/TrueHD exclusion, …) changed. |
| `generated_hdr_group_changed` | `Movies Generated Dynamic HDR Groups` changed (the -10000 DV/HDR10+ penalty). |
| `content_kind_scope_changed` | A Define's content-kind scope changed. |
| `define_metadata_changed` | Mapping-derived metadata (field, mode, tier, LQ fallbacks) changed consistently with the mapping. |
| `regex_or_source_gate_changed` | A raw-regex, release-title, obfuscation or Anime BD source-gate/case-flag condition changed. |
| `indirect_policy_effect` | A published rule consumes a changed Define. The detail names the rule, its score or action, and the profiles it ships in. |
| `expected_change_not_observed` | A changed group showed no behavior change in any real-engine probe. Check whether the classification is reachable. |

Indirect effects are listed by name. They include `Reject suspicious 4K upscale` (4K-upscale exemption), `Adaptive HD x265` (HD x265 exemption), `Unknown resolution`, `Reject bad 4K BluRay`/`Anime`, `Generated Dynamic HDR Penalty` and `Adaptive Low-Score Filtering`.

New trusted groups, new tier classifications and generated-HDR changes are always REVIEW_REQUIRED.

### BLOCKED (invalid output, missing evidence or unexpected structure)

| Code | Meaning |
|---|---|
| `input_unreadable` | An input is missing or is not valid JSON or share code. |
| `baseline_schema_changed`, `unexpected_baseline_field` | The baseline schema, mapping version or upstream URL changed, or there are unknown top-level fields. |
| `define_set_changed`, `mapping_mismatch` | A Define was added or removed, or generated metadata disagrees with the mapping. A sync never adds Defines. |
| `unknown_mode`, `unexpected_define_field` | A generator mode or a record/entry field the assessment does not understand. |
| `define_emptied` | A token-based Define has no tokens left. |
| `library_unparseable`, `library_define_set_mismatch`, `library_scope_mismatch` | The library cannot be parsed, or it disagrees with its baseline. |
| `trusted_define_mismatch` | `Trusted Release Groups` is not exactly the OR of every tier Define. |
| `library_not_reproducible` | The library does not re-render byte-for-byte from its baseline (detects hand edits). |
| `unexplained_library_change` | A rendered condition changed without a baseline change that explains it. |
| `consumer_define_missing` | A published rule references a Define the new library lacks. |
| `real_engine_regressions_failed`, `real_engine_regressions_not_run` | `scripts/test_streamnzb_compat.sh` failed or produced no result. |
| `engine_evidence_missing`, `engine_evidence_stale`, `engine_evidence_incomplete` | Planned probes have no results, results belong to different inputs, or some results are missing. |
| `unexpected_behavior_delta` | A probe changed behavior where no change was expected: a byte-identical Define, a near-miss group name, or the other content kind. |

## Real-engine evidence

The evidence comes from the real pinned StreamNZB/Jhin engine, never from re-running the generator:

1. **Compatibility suite.** `scripts/test_streamnzb_compat.sh` runs against the proposed library. Every existing fixture and production regression, including earlier sync regressions such as SumVision and monkee/CMRG, must pass.
2. **Old-vs-new probes.** For each added, removed or moved group (and a small sample of groups behind SAFE-candidate Defines), the assessment synthesizes releases for the Define's family and content kind:
   - `single`: the group's own tier/source shape (plus DV, HDR10+ and HDR10 variants for generated HDR, and Atmos/TrueHD shapes for the audio exclusions);
   - `cross-kind`: the same release in the sibling content kind (movie ↔ series, Anime movie ↔ Anime show);
   - `near-miss`: the group name with an extra character;
   - `4k-upscale`: a 2160p release next to a 1080p remux anchor;
   - `hd-x265`: a 1080p x265 release among seven 1080p AVC peers.

   Each probe is ranked through both published profiles twice, once with the old library and once with the new one. The assessment reports every difference in rank, kept/rejected state and matched rules.

A probe difference is *expected* only when the probed group changed in a Define that applies to that content kind (or a regex condition for that kind changed). Any other difference is `unexpected_behavior_delta` and blocks the change. The probe test records outcomes without asserting scores, so the evidence never encodes the generator's intent.

## Outputs

- **PR body:** the sync report followed by the assessment Markdown. The Markdown ends with an HTML comment `<!-- vidhin-sync-assessment {"decision": …, "reason_codes": […]} -->` for machine reading.
- **Workflow artifact** `vidhin-sync-assessment-<run id>`: `assessment.json` (decision, reasons, changes, effects, evidence and input SHA-256 digests), `assessment.md`, the probe plan and results, and the compatibility log.

The output is deterministic: the same inputs produce byte-identical JSON.

## Running it locally

```bash
./scripts/assess_vidhin_sync.sh [BASE_REF] [OUT_DIR]   # default BASE_REF=HEAD, OUT_DIR=mktemp
```

This compares the working-tree generated files against `BASE_REF`. For example, run `python3 scripts/sync_vidhin.py` first, then assess against `HEAD`.

## Limitations

- Only groups whose names are plain alphanumerics are probed. Other tokens are listed as not probeable and need manual review.
- Raw-regex, release-title and source-gate changes have no synthesized probes. They are always REVIEW_REQUIRED and rely on the compatibility suite.
- Probes use representative release shapes. They show that behavior changed, not that every real release behaves the same way.
- The tool assumes a sync PR. A PR that also changes the mapping or generator is assessed against the new mapping and may be BLOCKED by design.
