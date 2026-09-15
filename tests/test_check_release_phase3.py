#!/usr/bin/env python3
"""
Phase 3 focused tests: candidate published-release-collision fix, draft
selection, tag-first creation, release write/publish sequencing, compare
verification, notification correlation, and partial-failure boundaries.
Offline/fixture-based -- no live `gh` calls, no mutation of the
repository's real .release/ directory (temp-dir-isolated, same pattern
already used in tests/test_check_release.py / test_check_release_phase2.py).
"""

import inspect
import json
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import check_release as cr  # noqa: E402

REAL_COMPAT = cr.load_compatibility_baseline()
REAL_COUNTS = cr.load_current_counts()

MAIN_SHA = "8bd2455fc085f1e33c205d2092318b977dcdf3ce"
CANDIDATE_SHA = "e1383b0cdd361dd874da1c21c3bcea2fb55fc785"
PREVIOUS_STABLE = "6.0.1"

GOOD_NOTE = "# DraCuLa StreamNZB Template 9.9.9\n\nCurated public prose. Mentions 9.9.9.\n"
DRAFT_NOTE = f"# DraCuLa StreamNZB Template 9.9.9 {cr.DRAFT_HEADING_MARKER}\n\n{cr.DRAFT_NOTICE}\n\nBody.\n"

GOOD_PROVENANCE = {
    "schema_version": 1,
    "version": "9.9.9",
    "previous_version": PREVIOUS_STABLE,
    "prepared_from_sha": MAIN_SHA,
    "prepared_at_date": "2026-09-15",
    "compatibility": {
        "streamnzb_version": REAL_COMPAT["streamnzb_version"],
        "streamnzb_sha": REAL_COMPAT["streamnzb_sha"],
        "jhin_version": REAL_COMPAT["jhin_version"],
    },
    "expected_counts": dict(REAL_COUNTS),
}


def _with_release_dir(version, note_text, provenance, fn):
    original = cr.RELEASE_DIR
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_release_dir = Path(tmpdir)
        (tmp_release_dir / f"{version}.md").write_text(note_text, encoding="utf-8")
        (tmp_release_dir / f"{version}.json").write_text(json.dumps(provenance), encoding="utf-8")
        cr.RELEASE_DIR = tmp_release_dir
        try:
            return fn()
        finally:
            cr.RELEASE_DIR = original


def _finding(report, check):
    return next(f for f in report.findings if f.check == check)


def _run_candidate(version, sha, github, note_text=GOOD_NOTE, provenance=None):
    provenance = provenance or {**GOOD_PROVENANCE, "version": version}
    return _with_release_dir(version, note_text, provenance, lambda: cr.run_candidate(version, sha, github, offline=False))


# ---------------------------------------------------------------------------
# Candidate: the critical defect fix -- target-release-absent.
# ---------------------------------------------------------------------------

candidate_adapter = cr.FakeGithubAdapter(main_sha=CANDIDATE_SHA)

# a. valid candidate (no tag, no published release) passes target-release-absent.
report = _run_candidate("9.9.9", CANDIDATE_SHA, candidate_adapter)
f = _finding(report, "target-release-absent")
assert f.ok, f.render()

print("PASS: candidate passes target-release-absent when no published release exists yet for the version")

# b. published target release collision hard-fails, even though the tag
# check alone might otherwise look fine.
collision_adapter = cr.FakeGithubAdapter(
    main_sha=CANDIDATE_SHA,
    releases={
        "9.9.9": {
            "tag_name": "9.9.9",
            "target_commitish": CANDIDATE_SHA,
            "draft": False,
            "prerelease": False,
            "published_at": "2026-09-15T00:00:00Z",
            "body": "already out there",
        }
    },
)
report = _run_candidate("9.9.9", CANDIDATE_SHA, collision_adapter)
f = _finding(report, "target-release-absent")
assert not f.ok and f.severity == "error", f.render()
assert not report.passed

print("PASS: candidate hard-fails target-release-absent when a published release already exists for the requested version (the audited defect)")

# c. non-collision (a published release for a DIFFERENT version does not
# trip the check for this version).
other_version_adapter = cr.FakeGithubAdapter(
    main_sha=CANDIDATE_SHA,
    releases={
        "1.0.0": {
            "tag_name": "1.0.0", "target_commitish": "b" * 40, "draft": False,
            "prerelease": False, "published_at": "x", "body": "",
        }
    },
)
report = _run_candidate("9.9.9", CANDIDATE_SHA, other_version_adapter)
assert _finding(report, "target-release-absent").ok

print("PASS: candidate is unaffected by a published release belonging to a different version")

# d. invalid expected SHA fails input-format before any GitHub call.
report = cr.run_candidate("9.9.9", "not-a-sha", cr.FakeGithubAdapter(), offline=False)
f = _finding(report, "input-format")
assert not f.ok and f.severity == "error", f.render()

print("PASS: candidate fails input-format on a malformed expected SHA, before any GitHub call")

# e. main SHA mismatch fails.
wrong_main_adapter = cr.FakeGithubAdapter(main_sha="c" * 40)
report = _run_candidate("9.9.9", CANDIDATE_SHA, wrong_main_adapter)
f = _finding(report, "main-equals-candidate")
assert not f.ok and f.severity == "error", f.render()

print("PASS: candidate fails main-equals-candidate when GitHub main differs from the expected SHA")

# f. tag collision fails.
tag_collision_adapter = cr.FakeGithubAdapter(main_sha=CANDIDATE_SHA, tags={"9.9.9": {"sha": CANDIDATE_SHA, "type": "commit"}})
report = _run_candidate("9.9.9", CANDIDATE_SHA, tag_collision_adapter)
f = _finding(report, "tag-still-absent")
assert not f.ok and f.severity == "error", f.render()

print("PASS: candidate fails tag-still-absent when the tag already exists")

# g. provenance mismatch (stale prepared_from_sha, more than one first-parent hop) fails.
stale_provenance = {**GOOD_PROVENANCE, "prepared_from_sha": "f" * 40}
report = _run_candidate("9.9.9", CANDIDATE_SHA, candidate_adapter, provenance=stale_provenance)
f = _finding(report, "provenance-freshness")
assert not f.ok and f.severity == "error", f.render()

print("PASS: candidate fails provenance-freshness when prepared_from_sha doesn't resolve to a valid one-hop ancestor")

# h. curated-note failure.
report = _run_candidate("9.9.9", CANDIDATE_SHA, candidate_adapter, note_text=DRAFT_NOTE.replace("9.9.9", "9.9.9"))
f = _finding(report, "release-note-curated")
assert not f.ok and f.severity == "error", f.render()

print("PASS: candidate fails release-note-curated on an untouched generated draft")

# i. compatibility/count mismatch -- candidate mode itself doesn't snapshot
# -check provenance counts (that's prep-pr's job), but profile-define-counts
# must still be readable and consistent with the live repo.
report = _run_candidate("9.9.9", CANDIDATE_SHA, candidate_adapter)
assert _finding(report, "profile-define-counts").ok
assert _finding(report, "compatibility-baseline-consistent").ok

print("PASS: candidate's shared document checks (counts, compatibility baseline) pass against the live repo")

# j. multi-commit preparation PR provenance remains valid: real repo
# history where a 4-commit branch merged as exactly one first-parent hop
# (PR #30) -- proves check_preparation_provenance still passes with a
# real multi-commit branch, not just synthetic single-commit fixtures.
PR30_PREPARED_FROM = "f8356179ef61a8499c9fb841abe9a7dbfeb48e94"
PR30_MERGE = "1f4ecc79221c146f29695910cd36b6da07bfa7a8"
cr.check_preparation_provenance(PR30_PREPARED_FROM, PR30_MERGE)  # must not raise

print("PASS: check_preparation_provenance accepts a real multi-commit-branch/single-merge history (PR #30)")


# ---------------------------------------------------------------------------
# Read-only preflight never touches draft inventory (Section 2/5): a
# candidate run must never call matching_draft_releases at all.
# ---------------------------------------------------------------------------

class _DraftCallDetectingAdapter(cr.FakeGithubAdapter):
    def matching_draft_releases(self, tag_name):
        raise AssertionError("run_candidate must never call matching_draft_releases()")


_run_candidate("9.9.9", CANDIDATE_SHA, _DraftCallDetectingAdapter(main_sha=CANDIDATE_SHA))

print("PASS: run_candidate never calls matching_draft_releases (draft inventory is a privileged-token-only, publish-time concern)")

assert "matching_draft_releases" not in inspect.getsource(cr.run_candidate)
assert "create_tag_ref" not in inspect.getsource(cr.run_candidate)
assert "create_release" not in inspect.getsource(cr.run_candidate)
assert "update_release" not in inspect.getsource(cr.run_candidate)

print("PASS: run_candidate's source never references any privileged mutation method -- no privileged mutation before preflight success")


# ---------------------------------------------------------------------------
# Draft selection (Section 5).
# ---------------------------------------------------------------------------

TITLE = "DraCuLa StreamNZB Template 9.9.9"
BODY = "Curated body."

zero_draft_adapter = cr.FakeGithubAdapter()
release_id, created = cr.select_or_create_draft(zero_draft_adapter, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
assert created is True
assert len(zero_draft_adapter.created_releases) == 1
assert zero_draft_adapter.created_releases[0]["draft"] is True
assert zero_draft_adapter.created_releases[0]["tag_name"] == "9.9.9"

print("PASS: select_or_create_draft creates a controlled draft when zero exact-matching drafts exist")

one_draft_adapter = cr.FakeGithubAdapter(draft_releases_by_tag={"9.9.9": [{"id": 42, "name": "old", "created_at": "x"}]})
release_id, created = cr.select_or_create_draft(one_draft_adapter, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
assert created is False
assert release_id == 42
assert one_draft_adapter.created_releases == []

print("PASS: select_or_create_draft reuses the exact single matching draft without creating a second object")

dup_draft_adapter = cr.FakeGithubAdapter(
    draft_releases_by_tag={"9.9.9": [{"id": 1, "name": "a", "created_at": "x"}, {"id": 2, "name": "b", "created_at": "y"}]}
)
try:
    cr.select_or_create_draft(dup_draft_adapter, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
except cr.CheckError as exc:
    assert "1" in str(exc) and "2" in str(exc), str(exc)
else:
    raise AssertionError("duplicate exact-version drafts must fail closed")
assert dup_draft_adapter.created_releases == []

print("PASS: select_or_create_draft fails closed and reports every matching id when more than one exact-version draft exists")

unrelated_draft_adapter = cr.FakeGithubAdapter(
    draft_releases_by_tag={"6.0.2": [{"id": 1, "name": "a", "created_at": "x"}, {"id": 2, "name": "b", "created_at": "y"}]}
)
release_id, created = cr.select_or_create_draft(unrelated_draft_adapter, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
assert created is True  # the 6.0.2 duplicates must never interfere with publishing 9.9.9

print("PASS: unrelated duplicate drafts for a different version never interfere with draft selection for this version")


# ---------------------------------------------------------------------------
# Tag-first creation (Section 6).
# ---------------------------------------------------------------------------

tag_adapter = cr.FakeGithubAdapter()
result = cr.create_and_verify_tag(tag_adapter, "9.9.9", CANDIDATE_SHA)
assert tag_adapter.created_tag_refs == [("9.9.9", CANDIDATE_SHA)]
assert result == {"sha": CANDIDATE_SHA, "type": "commit"}
assert tag_adapter.tag_ref("9.9.9") == {"sha": CANDIDATE_SHA, "type": "commit"}

print("PASS: create_and_verify_tag creates exactly one lightweight tag ref with the correct name and exact SHA, resolving to object type 'commit'")

existing_tag_adapter = cr.FakeGithubAdapter(tags={"9.9.9": {"sha": "b" * 40, "type": "commit"}})
try:
    cr.create_and_verify_tag(existing_tag_adapter, "9.9.9", CANDIDATE_SHA)
except cr.CheckError as exc:
    assert "already exists" in str(exc)
else:
    raise AssertionError("an existing tag must never be silently moved")
assert existing_tag_adapter.created_tag_refs == []

print("PASS: create_and_verify_tag refuses to move/recreate an already-existing tag, and never even attempts creation")


class _WrongShaTagAdapter(cr.FakeGithubAdapter):
    def create_tag_ref(self, tag, sha):
        self.created_tag_refs.append((tag, sha))
        self._tags[tag] = {"sha": "f" * 40, "type": "commit"}  # simulate a server-side mismatch
        return dict(self._tags[tag])


try:
    cr.create_and_verify_tag(_WrongShaTagAdapter(), "9.9.9", CANDIDATE_SHA)
except cr.CheckError as exc:
    assert "resolved to" in str(exc)
else:
    raise AssertionError("a tag resolving to the wrong SHA after creation must fail")

print("PASS: create_and_verify_tag fails when the tag resolves to an unexpected SHA immediately after creation")


class _AnnotatedTagAdapter(cr.FakeGithubAdapter):
    def create_tag_ref(self, tag, sha):
        self.created_tag_refs.append((tag, sha))
        self._tags[tag] = {"sha": sha, "type": "tag"}  # simulate an annotated tag object
        return dict(self._tags[tag])


try:
    cr.create_and_verify_tag(_AnnotatedTagAdapter(), "9.9.9", CANDIDATE_SHA)
except cr.CheckError as exc:
    assert "annotated" in str(exc) or "object type" in str(exc)
else:
    raise AssertionError("an annotated tag object must fail create_and_verify_tag's directness check")

print("PASS: create_and_verify_tag fails if the created ref resolves to an annotated tag object instead of a direct commit")


class _FlakyTagReadAdapter(cr.FakeGithubAdapter):
    """Simulates GitHub's real eventual-consistency window: the very first
    tag_ref() read immediately after creation returns None (as a genuine
    transient 404 would), and only a later read observes the ref.
    create_and_verify_tag must retry a bounded number of times rather than
    treating a single missing read as a hard failure (CodeRabbit finding
    on PR #34)."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._created = False
        self._post_create_reads = 0

    def create_tag_ref(self, tag, sha):
        self._created = True
        return super().create_tag_ref(tag, sha)

    def tag_ref(self, tag):
        if self._created:
            self._post_create_reads += 1
            if self._post_create_reads == 1:
                return None
        return super().tag_ref(tag)


flaky_sleep_calls = []
flaky_adapter = _FlakyTagReadAdapter()
result = cr.create_and_verify_tag(
    flaky_adapter, "9.9.9", CANDIDATE_SHA, sleep_fn=lambda s: flaky_sleep_calls.append(s)
)
assert result == {"sha": CANDIDATE_SHA, "type": "commit"}
assert len(flaky_sleep_calls) == 1  # retried exactly once before succeeding

print("PASS: create_and_verify_tag retries a bounded number of times through a transient post-create 404 and still succeeds")


class _AlwaysMissingTagAdapter(cr.FakeGithubAdapter):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._created = False

    def create_tag_ref(self, tag, sha):
        self._created = True
        return super().create_tag_ref(tag, sha)

    def tag_ref(self, tag):
        if self._created:
            return None
        return super().tag_ref(tag)


always_missing_sleep_calls = []
try:
    cr.create_and_verify_tag(
        _AlwaysMissingTagAdapter(), "9.9.9", CANDIDATE_SHA, sleep_fn=lambda s: always_missing_sleep_calls.append(s)
    )
except cr.CheckError as exc:
    assert "verification attempts" in str(exc)
else:
    raise AssertionError("a tag that never becomes visible must still fail, not retry forever")
assert len(always_missing_sleep_calls) == 2  # bounded: 3 read attempts, 2 sleeps between them

print("PASS: create_and_verify_tag's retry is bounded -- a tag that never becomes visible still fails closed, not an infinite retry")


# ---------------------------------------------------------------------------
# Release write/publish sequencing (Section 7/8) via _assert_release_fields
# and write_and_publish_release.
# ---------------------------------------------------------------------------

GOOD_FIELDS = {"tag_name": "9.9.9", "name": TITLE, "body": BODY, "draft": False, "prerelease": False}
cr._assert_release_fields(dict(GOOD_FIELDS), "9.9.9", TITLE, BODY, draft=False, prerelease=False)  # must not raise

for field, bad_value, label in (
    ("name", "wrong title", "title mismatch"),
    ("body", "wrong body", "body mismatch"),
    ("draft", True, "draft mismatch"),
    ("prerelease", True, "prerelease mismatch"),
    ("tag_name", "1.0.0", "tag_name mismatch"),
):
    broken = {**GOOD_FIELDS, field: bad_value}
    try:
        cr._assert_release_fields(broken, "9.9.9", TITLE, BODY, draft=False, prerelease=False)
    except cr.CheckError:
        pass
    else:
        raise AssertionError(f"expected {label} to be detected")

print("PASS: _assert_release_fields detects title/body/draft/prerelease/tag_name mismatches individually")

publish_adapter = cr.FakeGithubAdapter()
release_id, _ = cr.select_or_create_draft(publish_adapter, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
published = cr.write_and_publish_release(publish_adapter, release_id, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
assert published["draft"] is False
assert published["prerelease"] is False
assert published["name"] == TITLE
assert published["body"] == BODY
assert len(publish_adapter.updated_releases) == 2  # stage-as-draft, then publish
assert publish_adapter.updated_releases[0][1]["draft"] is True
assert publish_adapter.updated_releases[1][1] == {"draft": False}

print("PASS: write_and_publish_release stages authoritative fields while draft, re-verifies, then publishes and re-verifies -- published state confirmed")


class _DriftAfterUpdateAdapter(cr.FakeGithubAdapter):
    """Simulates a server that silently returns a different `name` than
    what was written -- proves write_and_publish_release actually re-reads
    and validates rather than trusting its own update_release() call."""

    def release_by_id(self, release_id):
        release = super().release_by_id(release_id)
        release["name"] = "some other title"
        return release


drift_adapter = _DriftAfterUpdateAdapter()
release_id, _ = cr.select_or_create_draft(drift_adapter, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
try:
    cr.write_and_publish_release(drift_adapter, release_id, "9.9.9", CANDIDATE_SHA, TITLE, BODY)
except cr.CheckError as exc:
    assert "name=" in str(exc)
else:
    raise AssertionError("write_and_publish_release must detect a title mismatch on re-read")

print("PASS: write_and_publish_release detects drift between the intended and re-read release fields")


# ---------------------------------------------------------------------------
# Compare verification (Section 10).
# ---------------------------------------------------------------------------

PREV_TAG_SHA = "b" * 40

good_compare_adapter = cr.FakeGithubAdapter(
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "ahead", "ahead_by": 5, "behind_by": 0, "base_commit_sha": PREV_TAG_SHA}},
)
result = cr.verify_compare(good_compare_adapter, PREVIOUS_STABLE, "9.9.9")
assert result["behind_by"] == 0
assert good_compare_adapter.compare_calls == [(PREVIOUS_STABLE, "9.9.9")]

print("PASS: verify_compare passes for a valid range with behind_by=0 and a matching base commit")

wrong_base_adapter = cr.FakeGithubAdapter(
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "ahead", "ahead_by": 5, "behind_by": 0, "base_commit_sha": "c" * 40}},
)
try:
    cr.verify_compare(wrong_base_adapter, PREVIOUS_STABLE, "9.9.9")
except cr.CheckError as exc:
    assert "base commit" in str(exc)
else:
    raise AssertionError("a compare base commit that doesn't match the previous stable tag must fail")

print("PASS: verify_compare fails when the compare's base commit doesn't match the previous stable tag's target")

diverged_adapter = cr.FakeGithubAdapter(
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "diverged", "ahead_by": 3, "behind_by": 2, "base_commit_sha": PREV_TAG_SHA}},
)
try:
    cr.verify_compare(diverged_adapter, PREVIOUS_STABLE, "9.9.9")
except cr.CheckError as exc:
    assert "behind_by" in str(exc)
else:
    raise AssertionError("a diverged compare (behind_by != 0) must fail")

print("PASS: verify_compare fails on a diverged compare (behind_by != 0)")

pure_behind_adapter = cr.FakeGithubAdapter(
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "behind", "ahead_by": 0, "behind_by": 3, "base_commit_sha": PREV_TAG_SHA}},
)
try:
    cr.verify_compare(pure_behind_adapter, PREVIOUS_STABLE, "9.9.9")
except cr.CheckError as exc:
    assert "behind_by" in str(exc)
else:
    raise AssertionError("behind_by > 0 must fail even with status='behind'")

print("PASS: verify_compare fails when behind_by > 0")

# CodeRabbit finding (third pass on PR #34): an identical compare
# (status="identical", ahead_by=0) trivially satisfies behind_by == 0 and
# the base-commit check, but is not a valid new release -- the target
# must be a STRICT forward descendant of the previous stable tag.
identical_adapter = cr.FakeGithubAdapter(
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "identical", "ahead_by": 0, "behind_by": 0, "base_commit_sha": PREV_TAG_SHA}},
)
try:
    cr.verify_compare(identical_adapter, PREVIOUS_STABLE, "9.9.9")
except cr.CheckError as exc:
    detail = str(exc)
    assert "strict forward descendant" in detail
    assert "status='identical'" in detail
    assert "ahead_by=0" in detail
    assert "behind_by=0" in detail
else:
    raise AssertionError("an identical compare (ahead_by=0) must fail -- it is not a strict forward descendant")

print("PASS: verify_compare fails on an identical compare (status='identical', ahead_by=0) with a diagnostic naming status/ahead_by/behind_by")

api_failure_adapter = cr.FakeGithubAdapter(tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}})
try:
    cr.verify_compare(api_failure_adapter, PREVIOUS_STABLE, "9.9.9")  # no compare fixture configured -> simulated API failure
except cr.CheckError:
    pass
else:
    raise AssertionError("an unconfigured/failed compare call must propagate as CheckError, not silently pass")

print("PASS: verify_compare propagates a GitHub API failure as CheckError rather than degrading to a pass")


# ---------------------------------------------------------------------------
# Notification correlation (Section 11) -- bounded polling, WARN-only.
# ---------------------------------------------------------------------------

OPERATION_STARTED_AT = "2026-01-01T00:00:00+00:00"
EXPECTED_SHA = "d" * 40

success_run = {
    "id": 1, "head_sha": EXPECTED_SHA, "created_at": "2026-01-01T00:05:00+00:00",
    "status": "completed", "conclusion": "success", "html_url": "http://x/1", "event": "release",
}
notify_adapter = cr.FakeGithubAdapter(workflow_runs={"notify-release-discord.yml": [success_run]})
run = cr.correlate_publish_notification(notify_adapter, EXPECTED_SHA, OPERATION_STARTED_AT, sleep_fn=lambda s: None)
assert run == success_run
report = cr.Report()
cr.report_notification_outcome(report, run)
assert _finding(report, "publish-notification-correlated").ok

print("PASS: correlate_publish_notification finds and PASSes a matching successful run")

wrong_sha_run = {**success_run, "head_sha": "e" * 40}
notify_adapter = cr.FakeGithubAdapter(workflow_runs={"notify-release-discord.yml": [wrong_sha_run]})
run = cr.correlate_publish_notification(notify_adapter, EXPECTED_SHA, OPERATION_STARTED_AT, max_attempts=1, sleep_fn=lambda s: None)
assert run is None

print("PASS: correlate_publish_notification ignores a run whose head_sha doesn't match")

stale_run = {**success_run, "created_at": "2025-01-01T00:00:00+00:00"}  # before operation started
notify_adapter = cr.FakeGithubAdapter(workflow_runs={"notify-release-discord.yml": [stale_run]})
run = cr.correlate_publish_notification(notify_adapter, EXPECTED_SHA, OPERATION_STARTED_AT, max_attempts=1, sleep_fn=lambda s: None)
assert run is None

print("PASS: correlate_publish_notification ignores a run created before this publication operation started")

sleep_calls = []
empty_adapter = cr.FakeGithubAdapter(workflow_runs={})
run = cr.correlate_publish_notification(
    empty_adapter, EXPECTED_SHA, OPERATION_STARTED_AT, max_attempts=3, sleep_fn=lambda s: sleep_calls.append(s)
)
assert run is None
assert len(sleep_calls) == 2  # bounded: 3 attempts, sleeps between attempts only, never after the last

print("PASS: correlate_publish_notification polls a bounded number of attempts and stops (no run found -> None, not infinite)")

report = cr.Report()
cr.report_notification_outcome(report, None)
f = _finding(report, "publish-notification-correlated")
assert not f.ok and f.severity == "warning"

print("PASS: no matching notification run is WARN-only, never a hard failure")

failed_run = {**success_run, "conclusion": "failure"}
report = cr.Report()
cr.report_notification_outcome(report, failed_run)
f = _finding(report, "publish-notification-correlated")
assert not f.ok and f.severity == "warning"

print("PASS: a failed notification run is WARN-only, never a hard failure")

# CodeRabbit finding (second pass on PR #34): a matching run still
# queued/in_progress must not be returned prematurely -- its conclusion
# isn't final yet, so returning it early would report a misleading WARN
# for a notification that later actually succeeds. Poll again instead.
in_progress_run = {**success_run, "status": "in_progress", "conclusion": None}
sleep_calls_in_progress = []


class _EventuallyCompletesAdapter(cr.FakeGithubAdapter):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._calls = 0

    def list_workflow_runs(self, workflow_file, event=None):
        self._calls += 1
        if self._calls == 1:
            return [in_progress_run]
        return [success_run]


run = cr.correlate_publish_notification(
    _EventuallyCompletesAdapter(), EXPECTED_SHA, OPERATION_STARTED_AT,
    max_attempts=3, sleep_fn=lambda s: sleep_calls_in_progress.append(s),
)
assert run == success_run
assert len(sleep_calls_in_progress) == 1  # polled once more after seeing the in_progress run

print("PASS: correlate_publish_notification keeps polling a matching in_progress/queued run instead of returning it prematurely")

# A transient query failure (a real GhCliAdapter CheckError) must not
# abort the bounded poll -- it's treated as "no match this attempt".
class _FlakyQueryAdapter(cr.FakeGithubAdapter):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._calls = 0

    def list_workflow_runs(self, workflow_file, event=None):
        self._calls += 1
        if self._calls == 1:
            raise cr.CheckError("simulated transient GitHub API failure")
        return [success_run]


flaky_query_sleeps = []
run = cr.correlate_publish_notification(
    _FlakyQueryAdapter(), EXPECTED_SHA, OPERATION_STARTED_AT,
    max_attempts=3, sleep_fn=lambda s: flaky_query_sleeps.append(s),
)
assert run == success_run
assert len(flaky_query_sleeps) == 1

print("PASS: correlate_publish_notification survives a transient list_workflow_runs() query failure and keeps polling")


# ---------------------------------------------------------------------------
# run_publish end-to-end + partial-failure boundaries (Sections 9/12).
# ---------------------------------------------------------------------------

def _run_publish(version, expected_sha, github, note_text=GOOD_NOTE, provenance=None):
    provenance = provenance or {**GOOD_PROVENANCE, "version": version}
    return _with_release_dir(
        version, note_text, provenance,
        lambda: cr.run_publish(version, expected_sha, github, sleep_fn=lambda s: None),
    )


def _happy_adapter():
    return cr.FakeGithubAdapter(
        main_sha=CANDIDATE_SHA,
        tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
        compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "ahead", "ahead_by": 1, "behind_by": 0, "base_commit_sha": PREV_TAG_SHA}},
        workflow_runs={"notify-release-discord.yml": [
            {"id": 1, "head_sha": CANDIDATE_SHA, "created_at": "2099-01-01T00:00:00+00:00",
             "status": "completed", "conclusion": "success", "html_url": "http://x", "event": "release"},
        ]},
    )


happy_gh = _happy_adapter()
report = _run_publish("9.9.9", CANDIDATE_SHA, happy_gh)
assert report.passed, report.render()
assert len(happy_gh.created_tag_refs) == 1
assert len(happy_gh.created_releases) == 1
assert happy_gh.created_releases[0]["draft"] is True
assert happy_gh.updated_releases[-1][1] == {"draft": False}

print("PASS: run_publish succeeds end to end -- draft created, tag created+verified, release staged+published+verified, compare verified, notification correlated")

# Defense-in-depth re-check (CodeRabbit finding on PR #34): if main has
# moved past expected_sha since the read-only candidate preflight ran
# (a narrow but real window once the privileged token is minted),
# run_publish must abort before any mutation -- not just trust that the
# earlier preflight step is still valid.
main_drifted_adapter = _happy_adapter()
main_drifted_adapter._main_sha = "f" * 40
try:
    _run_publish("9.9.9", CANDIDATE_SHA, main_drifted_adapter)
except cr.CheckError as exc:
    assert "main has moved" in str(exc)
else:
    raise AssertionError("run_publish must abort if live main no longer equals expected_sha")
assert main_drifted_adapter.created_tag_refs == []
assert main_drifted_adapter.created_releases == []

print("PASS: run_publish re-checks live main == expected_sha before any mutation, aborting closed if main has moved since the candidate preflight")

# GitHub/API errors fail closed before any irreversible mutation: a
# duplicate-draft collision must abort before tag creation.
dup_adapter = _happy_adapter()
dup_adapter._draft_releases_by_tag["9.9.9"] = [{"id": 1, "name": "a", "created_at": "x"}, {"id": 2, "name": "b", "created_at": "y"}]
try:
    _run_publish("9.9.9", CANDIDATE_SHA, dup_adapter)
except cr.CheckError:
    pass
else:
    raise AssertionError("duplicate exact-version drafts must abort run_publish before any mutation")
assert dup_adapter.created_tag_refs == []
assert dup_adapter.created_releases == []

print("PASS: run_publish fails closed on duplicate exact-version drafts before any GitHub mutation (safe to fix and rerun)")

# Tag creation failure prevents release mutation entirely.
tag_fail_adapter = _happy_adapter()
tag_fail_adapter._tags["9.9.9"] = {"sha": "already-there".ljust(40, "0"), "type": "commit"}
try:
    _run_publish("9.9.9", CANDIDATE_SHA, tag_fail_adapter)
except cr.CheckError as exc:
    assert "already exists" in str(exc)
else:
    raise AssertionError("an already-existing tag must abort before any release write")
assert tag_fail_adapter.updated_releases == []  # draft was created/selected, but never staged/published

print("PASS: a tag-creation failure prevents any release-object mutation (draft may exist, but is never staged/published)")

# Post-tag release failure does not delete/move the tag, and reports a
# clear "do not auto-recover" message.
class _FailPublishAdapter(cr.FakeGithubAdapter):
    def update_release(self, release_id, **fields):
        if fields.get("draft") is False:
            raise cr.CheckError("simulated GitHub API failure on publish")
        return super().update_release(release_id, **fields)


fail_publish_adapter = _FailPublishAdapter(
    main_sha=CANDIDATE_SHA,
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "ahead", "ahead_by": 1, "behind_by": 0, "base_commit_sha": PREV_TAG_SHA}},
)
try:
    _run_publish("9.9.9", CANDIDATE_SHA, fail_publish_adapter)
except cr.CheckError as exc:
    assert "POST-TAG FAILURE" in str(exc)
    assert "do not" in str(exc).lower() and "tag" in str(exc).lower()
else:
    raise AssertionError("a post-tag release-publish failure must raise, not silently continue")
assert fail_publish_adapter.created_tag_refs == [("9.9.9", CANDIDATE_SHA)]
assert fail_publish_adapter.tag_ref("9.9.9") == {"sha": CANDIDATE_SHA, "type": "commit"}  # tag left exactly as created, never moved/deleted

print("PASS: a post-tag release-publish failure raises a loud 'POST-TAG FAILURE' CheckError and never moves/deletes the already-created tag")

# Post-publication verification failure does not attempt rollback: the
# release publishes successfully, but a re-read drift is reported, not
# auto-fixed, and run_publish returns (does not raise) so the caller can
# still see every other check's outcome.
class _DriftAfterPublishAdapter(cr.FakeGithubAdapter):
    """Passes write_and_publish_release's own immediate re-read (the first
    draft=false read), so the release genuinely publishes successfully --
    only a SECOND, independent read (run_publish's separate
    post-publication-release-verification step) observes drift. Proves
    that step is a real, distinct re-verification, not just reusing
    write_and_publish_release's own result."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._published_reads = 0

    def release_by_id(self, release_id):
        release = super().release_by_id(release_id)
        if release["draft"] is False:
            self._published_reads += 1
            if self._published_reads > 1:
                release["name"] = "drifted title"
        return release


drift_publish_adapter = _DriftAfterPublishAdapter(
    main_sha=CANDIDATE_SHA,
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "ahead", "ahead_by": 1, "behind_by": 0, "base_commit_sha": PREV_TAG_SHA}},
    workflow_runs={"notify-release-discord.yml": [
        {"id": 1, "head_sha": CANDIDATE_SHA, "created_at": "2099-01-01T00:00:00+00:00",
         "status": "completed", "conclusion": "success", "html_url": "http://x", "event": "release"},
    ]},
)
report = _run_publish("9.9.9", CANDIDATE_SHA, drift_publish_adapter)
assert not report.passed
f = _finding(report, "post-publication-release-verification")
assert not f.ok and f.severity == "error"
assert "HUMAN INSPECTION" in f.detail
# No automatic mutation attempted beyond the two normal update_release calls
# (stage-as-draft, publish) -- drift detection itself never re-mutates.
assert len(drift_publish_adapter.updated_releases) == 2

print("PASS: a post-publication verification failure is reported loudly (report.passed=False) but never triggers automatic mutation -- run_publish returns rather than raising, since the release is already public")

# CodeRabbit finding (second pass on PR #34): a real GhCliAdapter read
# failure during post-publication tag_ref()/release() re-verification
# must not escape run_publish() as a raw, uncaught exception -- the
# release is already public by this point, so this is exactly the same
# "needs human inspection, never auto-mutate" reporting boundary as any
# other post-publication drift, not a fatal abort.
class _PostPublicationQueryFailureAdapter(cr.FakeGithubAdapter):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._tag_ref_calls = 0
        self._release_calls = 0

    def tag_ref(self, tag):
        self._tag_ref_calls += 1
        # Call 1 = create_and_verify_tag's pre-create existence check,
        # call 2 = its post-create verification, call 3 = run_publish's
        # own post-publication re-verification -- fail only that third one.
        if self._tag_ref_calls == 3:
            raise cr.CheckError("simulated transient GitHub API failure reading tag")
        return super().tag_ref(tag)

    def release(self, tag):
        self._release_calls += 1
        # Call 1 = run_publish's defense-in-depth pre-mutation collision
        # check, call 2 = its post-publication re-verification -- fail
        # only that second one.
        if self._release_calls == 2:
            raise cr.CheckError("simulated transient GitHub API failure reading release")
        return super().release(tag)


query_failure_adapter = _PostPublicationQueryFailureAdapter(
    main_sha=CANDIDATE_SHA,
    tags={PREVIOUS_STABLE: {"sha": PREV_TAG_SHA, "type": "commit"}},
    compares={(PREVIOUS_STABLE, "9.9.9"): {"status": "ahead", "ahead_by": 1, "behind_by": 0, "base_commit_sha": PREV_TAG_SHA}},
    workflow_runs={"notify-release-discord.yml": [
        {"id": 1, "head_sha": CANDIDATE_SHA, "created_at": "2099-01-01T00:00:00+00:00",
         "status": "completed", "conclusion": "success", "html_url": "http://x", "event": "release"},
    ]},
)
report = _run_publish("9.9.9", CANDIDATE_SHA, query_failure_adapter)  # must not raise
assert not report.passed
tag_finding = _finding(report, "post-publication-tag-verification")
assert not tag_finding.ok and "could not re-fetch tag" in tag_finding.detail
release_finding = _finding(report, "post-publication-release-exists")
assert not release_finding.ok and "could not re-fetch release" in release_finding.detail
# Everything downstream of the failed reads still runs -- a read failure
# on one check must not abort the rest of the report.
assert _finding(report, "compare-verification").ok
assert _finding(report, "publish-notification-correlated").ok

print("PASS: a post-publication tag_ref()/release() API failure is reported as a finding, not raised, and doesn't block compare-verification/notification-correlation from still running")


# ---------------------------------------------------------------------------
# GhCliAdapter pagination (CodeRabbit third pass on PR #34): the
# authoritative privileged draft query must see every release page, not
# just GitHub's default single page. Constructs a real GhCliAdapter
# instance (bypassing __init__'s gh-auth check) with `_api` monkeypatched
# to serve canned pages -- proves the pagination mechanism itself
# (explicit page=N/per_page=N query construction via _api_paginated), not
# just FakeGithubAdapter behavior, which has no pagination concept at all.
# ---------------------------------------------------------------------------

def _paged_gh_adapter(pages):
    """pages: list of lists of release dicts, one list per page."""
    adapter = cr.GhCliAdapter.__new__(cr.GhCliAdapter)
    adapter.repo = "d4s87/streamnzb-template"
    requested_paths = []

    def fake_api(path, allow_404=False):
        requested_paths.append(path)
        _base, _, query = path.partition("?")
        params = dict(p.split("=", 1) for p in query.split("&") if p)
        page_number = int(params.get("page", "1"))
        if page_number > len(pages):
            return []
        return pages[page_number - 1]

    adapter._api = fake_api
    adapter._requested_paths = requested_paths
    return adapter


def _fake_release(id_, tag_name, draft):
    return {
        "id": id_, "tag_name": tag_name, "target_commitish": "main",
        "name": "", "body": "", "draft": draft, "prerelease": False, "published_at": None,
    }


class _FixedPageSizeAdapter:
    """Thin proxy forcing a small per_page through matching_draft_releases
    so select_or_create_draft (which calls it with no per_page override)
    can be exercised against genuinely multi-page data in a test, without
    changing select_or_create_draft's own signature."""

    def __init__(self, inner, per_page):
        self._inner = inner
        self._per_page = per_page

    def matching_draft_releases(self, tag_name):
        return self._inner.matching_draft_releases(tag_name, per_page=self._per_page)

    def __getattr__(self, name):
        return getattr(self._inner, name)


# a. matching exact-version draft found on page 2+.
page1 = [_fake_release(1, "1.0.0", False), _fake_release(2, "1.0.1", False)]
page2 = [_fake_release(3, "9.9.9", True)]
gh_adapter = _paged_gh_adapter([page1, page2])
matches = gh_adapter.matching_draft_releases("9.9.9", per_page=2)
assert [m["id"] for m in matches] == [3]
assert any("page=1" in p for p in gh_adapter._requested_paths)
assert any("page=2" in p for p in gh_adapter._requested_paths)

print("PASS: GhCliAdapter.matching_draft_releases finds an exact-version draft on page 2+, requesting pages explicitly (page=1, page=2)")

# b. duplicate exact-version drafts split across pages: both seen, and
# select_or_create_draft (called exactly as run_publish calls it) fails closed.
page1 = [_fake_release(1, "9.9.9", True)]
page2 = [_fake_release(2, "9.9.9", True)]
gh_adapter = _paged_gh_adapter([page1, page2])
assert [m["id"] for m in gh_adapter.matching_draft_releases("9.9.9", per_page=1)] == [1, 2]
try:
    cr.select_or_create_draft(_FixedPageSizeAdapter(gh_adapter, per_page=1), "9.9.9", CANDIDATE_SHA, TITLE, BODY)
except cr.CheckError as exc:
    assert "1" in str(exc) and "2" in str(exc)
else:
    raise AssertionError("duplicate exact-version drafts split across pages must fail closed")

print("PASS: duplicate exact-version drafts split across different pages are both detected by pagination and fail closed via select_or_create_draft")

# c. unrelated drafts on later pages do not interfere with a different
# version's exact match.
page1 = [_fake_release(1, "1.0.0", False)]
page2 = [_fake_release(2, "6.0.2", True)]  # unrelated version/tag
gh_adapter = _paged_gh_adapter([page1, page2])
assert gh_adapter.matching_draft_releases("9.9.9", per_page=1) == []

print("PASS: unrelated drafts on later pages never interfere with a different version's exact match")

# d. zero exact-version matches across ALL pages is a genuine empty
# result verified against multiple real, non-matching pages (not just a
# single-page absence) -- select_or_create_draft (already proven
# adapter-agnostic elsewhere) then creates a controlled draft.
page1 = [_fake_release(1, "1.0.0", False), _fake_release(2, "1.0.1", True)]
page2 = [_fake_release(3, "6.0.2", True)]
gh_adapter = _paged_gh_adapter([page1, page2])
assert gh_adapter.matching_draft_releases("9.9.9", per_page=2) == []
assert any("page=1" in p for p in gh_adapter._requested_paths)
assert any("page=2" in p for p in gh_adapter._requested_paths)

print("PASS: zero exact-version matches confirmed across multiple real non-matching pages, feeding select_or_create_draft's existing controlled-draft-creation path")
