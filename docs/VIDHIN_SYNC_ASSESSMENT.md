# Vidhin sync assessment

Every Vidhin synchronization PR carries an advisory assessment with a machine-readable decision: **SAFE**, **REVIEW_REQUIRED** or **BLOCKED**.

The assessment never merges, approves or labels a PR, and it does not decide release impact. The copy in the PR body is advisory. The [`Validate Vidhin sync PR` check](#pr-head-validation) re-runs the assessment against the exact PR head and is the authoritative assessment result. A passing check is evidence, not authorization: it does not by itself make a PR safe to merge automatically (see [Security model](#security-model)). Human review and human release-impact labeling stay exactly as they were.

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
| `define_metadata_changed` | Mapping-derived metadata (field, mode, tier, LQ exact groups, LQ fallbacks, LQ case-insensitive group overrides) changed consistently with the mapping. |
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

## PR-head validation

`.github/workflows/validate-vidhin-sync-pr.yml` runs the check **`Validate Vidhin sync PR`** on every PR into `main`. It ignores the assessment in the PR body, bot comments and artifacts from the sync run. It produces its own evidence for the exact PR head commit.

Implementation: `scripts/validate_vidhin_sync_pr.py`, tested by `tests/test_validate_vidhin_sync_pr.py`.

### What it does

1. **Classify.** Only a PR from the branch `chore/vidhin-sync` is validated. Every other PR finishes as `NOT_APPLICABLE` and passes. This happens in the job's first step, which needs no base code. The job has no job-level `if:`, because a skipped run would count as success and could mask an earlier failure for the same commit. For the same reason, a non-sync PR fails when its head SHA is the live `chore/vidhin-sync` head: check results attach to commits, so its pass would hide the sync PR's result. A failure to read that head also fails the check.
2. **Identity and scope.** A `chore/vidhin-sync` PR must be authored by `streamnzb-template-prepare-release[bot]` (type `Bot`) and come from this repository, not a fork. The PR base must be an ancestor of the head, so the PR is up to date. The direct base-to-head tree diff may only modify the three generated files (`generated/vidhin-defines.json`, `generated/vidhin-sync-report.md`, `generated/streamnzb-defines.txt`), in place, with mode `100644`. Any other path, addition, deletion or mode change is `unauthorized_change`. An empty diff is `empty_sync`.
3. **Reproduce.** The upstream records stored in the head baseline are fed through the base commit's `sync_vidhin.py`, using the base mapping and baseline. The head baseline, the report and the Define Library must match the output byte for byte. The only exception is the value of the top-level `generated_at_utc`, which must be a second-precision UTC timestamp in the generator's own format. Formatting, key order, escapes and duplicate keys must match exactly; a duplicate key fails the run. Otherwise the result is `generator_mismatch` or `generator_failed`.
4. **Assess.** `scripts/assess_vidhin_sync.sh` runs from the base checkout against the base commit. It uses the head's generated files and runs the pinned StreamNZB compatibility suite and real-engine probes.
5. **Verify the evidence** before deciding:
   - **Checkout integrity:** the checkout is still the base commit. The generated files, mapping, profiles and compatibility script still match their head/base git blobs, and no other file or git hook appeared (`checkout_modified`).
   - **Engine revision:** `compat.log` names exactly the `STREAMNZB_REF` pinned in the base `scripts/test_streamnzb_compat.sh` (`engine_revision_mismatch`, `compat_evidence_missing`). The pass/fail result comes from the log, not from the shell exit status.
   - **Independent re-run:** the semantic assessment is run again on the verified inputs and probe results. Its output must equal `assessment.json` byte for byte. Any edited decision, reason or probe result, or a SAFE result the compatibility log contradicts, is `assessment_contradictory`. A missing or unreadable `assessment.json` is `assessment_missing`.
   - **Freshness:** after the engine run, the live PR head and base-branch tip must still equal the commits that were validated (`stale_pr_head`, `stale_base`).
6. **Decide.** Any failure above is **BLOCKED**. Otherwise the decision is the re-verified assessment decision.

| Decision | Check result |
|---|---|
| `NOT_APPLICABLE` | pass |
| `SAFE` | pass |
| `REVIEW_REQUIRED` | pass. The evidence is valid, but the behavior change still needs a human. |
| `BLOCKED` | **fail** |

The job summary shows the decision. The artifact `vidhin-pr-validation-<run id>-<attempt>` holds `decision.json` (schema 1: decision, PR number, head/base SHAs, author, reasons, re-verified assessment decision) and all evidence.

### Security model

- **No untrusted code.** The trigger is plain `pull_request` (no `pull_request_target`). Permissions are `contents: read` and `pull-requests: read`. The workflow uses no secrets, and the checkout uses `persist-credentials: false`. All code that runs comes from the base commit. The head contributes only three data files. PR-controlled values (branch, author, SHAs) reach scripts only through environment variables.
- **The workflow file comes from the PR merge ref.** This is how `pull_request` works: a PR that edits the workflow could replace the check with its own. The scope rule rejects any such PR, but only an unmodified workflow enforces that rule. So a passing check is trustworthy only for a PR whose changed files are within the generated scope. A passing check, a matching branch name and a bot author are not enough to authorize a merge. Any future auto-merge must verify all of these itself, from trusted code, at merge time:
  - **PR identity:** the PR number, head branch `chore/vidhin-sync` and author `streamnzb-template-prepare-release[bot]`;
  - **source repository:** the head repository is this repository, not a fork;
  - **changed-file scope:** the PR's current file list, read from the API, is limited to the three generated files;
  - **current SHAs:** the PR head and the base-branch tip right now equal the head and base SHAs in `decision.json`;
  - **trusted evidence:** the check run and `decision.json` come from this workflow's `pull_request` run for that PR (see the next point), and the decision is `SAFE`.
- **Check results belong to commits, not PRs.** For a given name, the newest check run on a SHA is what counts. The classify step stops another PR that reuses the live sync head from posting a pass over the sync PR's result. It cannot stop the reverse. Anyone can open an unrelated PR, including from a fork, whose head is the sync PR's head commit. That PR's run then fails (as `fork_head` or in the classify step), and it may become the newest result for the commit, turning a valid sync PR's check red. This can delay a merge, but it cannot turn a BLOCKED sync into a pass.
  - **Diagnose:** in the sync PR's **Checks** tab, open the `Validate Vidhin sync PR` run. Check that its workflow run lists the sync PR as its pull request, for example with `gh run view <run id> --json event,headBranch,headSha`. A failing run whose head branch is not `chore/vidhin-sync`, or whose summary shows the classify error, is interference. So is a run from a fork. `gh api repos/d4s87/streamnzb-template/commits/<sha>/check-runs` lists every run on the commit.
  - **Recover:** re-run the sync PR's own `Validate Vidhin sync PR` run (**Re-run jobs**, or `gh run rerun <run id>`). Its new result becomes the newest result for the commit. Close the interfering PR if it has no other purpose.
  - **Merge controller:** a future merge controller must select the check run whose workflow run was triggered by the sync PR's own `pull_request` event (matching PR number, head branch and head SHA). It must verify that run's conclusion and `decision.json`, never just a check with a matching name or SHA. This needs no privileged workflow.
- **Bootstrap.** The validator runs from the base commit. A `chore/vidhin-sync` PR whose base predates this workflow fails the check, and the next sync from `main` validates normally. This PR's own run passes as `NOT_APPLICABLE`.
- **Time-of-check/time-of-use.** Reruns re-read the live head and base and block when either moved. A new push starts a new run, and `cancel-in-progress` cancels the old one. A base change after a passing run leaves that run's green check in place, so the merge gate must require branches to be up to date (below).

### Making it a required check (manual)

The check works on every PR, but **it is not enforced yet**. On 2026-11-01, `main` had no branch protection and no rulesets.

**Dependabot tradeoff: decide before applying.** Strict branch freshness ("Require branches to be up to date before merging") is the recommended security setting, because it stops a sync validated against an older `main` from merging. It changes how the existing Dependabot automation behaves:

- `.github/workflows/dependabot-automerge.yml` merges with a direct `gh pr merge --squash` (not `--auto`) as soon as "Validate StreamNZB Template" passes on a Dependabot PR. Repository auto-merge is disabled (`allow_auto_merge: false`).
- Dependabot opens one grouped GitHub Actions PR per week. If anything lands on `main` after that PR's branch was created, the PR is out of date. With strict freshness, the direct merge is then rejected, the auto-merge run fails and the PR stays open.
- Dependabot rebases on its own only when its PR conflicts, not when it is merely behind. To recover, comment `@dependabot rebase` on the PR (or use **Update branch**). The new push re-runs Validate, and its completion triggers the auto-merge again.

Do not apply the ruleset below until the maintainer has either accepted this tradeoff (manual Dependabot rebases when `main` moved) or implemented a separately reviewed Dependabot adaptation. This PR changes neither the Dependabot workflow nor the repository's auto-merge setting.

When that decision is made, a repository administrator adds the rule after this workflow is on `main`:

1. Go to **Settings → Rules → Rulesets → New ruleset → New branch ruleset**.
2. Name it, for example, `main protection`. Set **Enforcement status** to **Active**.
3. Under **Target branches**, choose **Add target → Include default branch**.
4. Enable **Require a pull request before merging**. Set required approvals as wanted (0 keeps today's self-merge flow).
5. Enable **Require status checks to pass**, then:
   - enable **Require branches to be up to date before merging**;
   - add the check **`Validate Vidhin sync PR`** with source **GitHub Actions**;
   - optionally add `Validate profile and Define library` and `Validate PR title, body, and branch`.
6. Keep **Block force pushes** and **Restrict deletions** enabled (the defaults).
7. Leave the bypass list empty, or add only the repository admin role with **For pull requests only**. Any bypass actor can merge a BLOCKED sync.

Equivalent API call (admin token):

```bash
gh api -X POST repos/d4s87/streamnzb-template/rulesets --input - <<'JSON'
{
  "name": "main protection",
  "target": "branch",
  "enforcement": "active",
  "conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}},
  "rules": [
    {"type": "deletion"},
    {"type": "non_fast_forward"},
    {"type": "pull_request", "parameters": {"required_approving_review_count": 0,
      "dismiss_stale_reviews_on_push": false, "require_code_owner_review": false,
      "require_last_push_approval": false, "required_review_thread_resolution": false}},
    {"type": "required_status_checks", "parameters": {"strict_required_status_checks_policy": true,
      "required_status_checks": [{"context": "Validate Vidhin sync PR", "integration_id": 15368}]}}
  ]
}
JSON
```

`integration_id` 15368 is the GitHub Actions app. It stops a status that some other integration posts under the same name from satisfying the rule. After enabling the ruleset, check `gh api repos/d4s87/streamnzb-template/rules/branches/main`. Then confirm that a PR's merge box lists `Validate Vidhin sync PR` as **Required**.

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
