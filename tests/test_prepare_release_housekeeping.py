#!/usr/bin/env python3

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

from check_release import CheckError  # noqa: E402
from prepare_release_housekeeping import (  # noqa: E402
    build_provenance,
    generate_release_note,
    rollover_changelog,
    update_readme_version,
)


# ---------------------------------------------------------------------------
# update_readme_version
# ---------------------------------------------------------------------------

README_FIXTURE = (
    "# Title\n"
    "\n"
    "Intro text.\n"
    "\n"
    "**Current version: 6.0.1**  \n"
    "**Compatibility: StreamNZB 6.1.0 / Jhin 0.7.1**\n"
    "\n"
    "More text.\n"
)

result = update_readme_version(README_FIXTURE, "6.0.2")
assert "**Current version: 6.0.2**  \n" in result
assert "**Current version: 6.0.1**" not in result
assert result.count("**Current version:") == 1
# Hard-break trailing two spaces preserved byte-for-byte.
assert "**Current version: 6.0.2**  \n**Compatibility:" in result
# Everything else untouched.
assert result.replace("6.0.2", "6.0.1") == README_FIXTURE

print("PASS: update_readme_version exact one-line replacement, hard-break preserved")

try:
    update_readme_version("no anchor here", "6.0.2")
except CheckError as exc:
    assert "not found" in str(exc)
else:
    raise AssertionError("missing README anchor was not detected")

print("PASS: update_readme_version missing anchor fails closed")

DUPLICATE_README = README_FIXTURE + "\n**Current version: 6.0.1**  \n"
try:
    update_readme_version(DUPLICATE_README, "6.0.2")
except CheckError as exc:
    assert "expected exactly 1" in str(exc)
else:
    raise AssertionError("duplicate README anchor was not detected")

print("PASS: update_readme_version duplicate anchor fails closed")


# ---------------------------------------------------------------------------
# rollover_changelog
# ---------------------------------------------------------------------------

SLUG = "d4s87/streamnzb-template"

CHANGELOG_EMPTY_UNRELEASED = (
    "# Changelog\n"
    "\n"
    f"## [Unreleased](https://github.com/{SLUG}/compare/6.0.1...HEAD)\n"
    "\n"
    "Changes in this section are under development and are not part of the latest stable release.\n"
    "\n"
    f"## [6.0.1](https://github.com/{SLUG}/compare/6.0.0...6.0.1) (2026-09-15)\n"
    "\n"
    "6.0.1 body text.\n"
    "\n"
    f"## [6.0.0](https://github.com/{SLUG}/compare/v5.2...6.0.0) (2026-09-14)\n"
    "\n"
    "6.0.0 body text.\n"
)

new_text, moved_body = rollover_changelog(
    CHANGELOG_EMPTY_UNRELEASED, "6.0.2", "6.0.1", "2026-09-20", SLUG
)

assert moved_body == ""
assert f"## [Unreleased](https://github.com/{SLUG}/compare/6.0.2...HEAD)" in new_text
assert (
    f"## [6.0.2](https://github.com/{SLUG}/compare/6.0.1...6.0.2) (2026-09-20)"
    in new_text
)
# Historical sections (6.0.1 and 6.0.0) are byte-identical to the original,
# just shifted down -- prove by slicing from the 6.0.1 header in both texts.
old_tail = CHANGELOG_EMPTY_UNRELEASED[CHANGELOG_EMPTY_UNRELEASED.index("## [6.0.1]"):]
new_tail = new_text[new_text.index("## [6.0.1]"):]
assert old_tail == new_tail, "historical sections must be byte-identical"

print("PASS: rollover_changelog clean rollover with empty Unreleased body, historical sections untouched")

CHANGELOG_WITH_BODY = (
    "# Changelog\n"
    "\n"
    f"## [Unreleased](https://github.com/{SLUG}/compare/6.0.1...HEAD)\n"
    "\n"
    "Changes in this section are under development and are not part of the latest stable release.\n"
    "\n"
    "### Bug Fixes\n"
    "\n"
    "- **scoring:** fixed something.\n"
    "- **formatter:** fixed something else.\n"
    "\n"
    f"## [6.0.1](https://github.com/{SLUG}/compare/6.0.0...6.0.1) (2026-09-15)\n"
    "\n"
    "6.0.1 body text.\n"
)

new_text2, moved_body2 = rollover_changelog(
    CHANGELOG_WITH_BODY, "6.0.2", "6.0.1", "2026-09-20", SLUG
)

assert moved_body2 == (
    "### Bug Fixes\n\n- **scoring:** fixed something.\n- **formatter:** fixed something else."
)
assert moved_body2 in new_text2
# The new Unreleased section itself must NOT carry the moved bullets.
new_unreleased_region = new_text2[: new_text2.index("## [6.0.2]")]
assert "### Bug Fixes" not in new_unreleased_region
# Nothing was synthesized: the moved body is exactly the old Unreleased body,
# not a rephrasing/summary.
assert new_text2.count("### Bug Fixes") == 1
old_tail2 = CHANGELOG_WITH_BODY[CHANGELOG_WITH_BODY.index("## [6.0.1]"):]
new_tail2 = new_text2[new_text2.index("## [6.0.1]"):]
assert old_tail2 == new_tail2

print("PASS: rollover_changelog moves non-empty Unreleased body verbatim, no synthesis")

# Malformed markers fail closed.
try:
    rollover_changelog(CHANGELOG_EMPTY_UNRELEASED.replace("## [Unreleased]", "## [WIP]"), "6.0.2", "6.0.1", "2026-09-20", SLUG)
except CheckError as exc:
    assert "expected exactly one Unreleased header" in str(exc)
else:
    raise AssertionError("missing Unreleased header was not detected")

DUPLICATE_HEADER = CHANGELOG_EMPTY_UNRELEASED + f"\n## [Unreleased](https://github.com/{SLUG}/compare/6.0.1...HEAD)\n"
try:
    rollover_changelog(DUPLICATE_HEADER, "6.0.2", "6.0.1", "2026-09-20", SLUG)
except CheckError as exc:
    assert "found 2" in str(exc)
else:
    raise AssertionError("duplicate Unreleased header was not detected")

BROKEN_NOTICE = CHANGELOG_EMPTY_UNRELEASED.replace(
    "Changes in this section are under development and are not part of the latest stable release.",
    "Some other notice.",
)
try:
    rollover_changelog(BROKEN_NOTICE, "6.0.2", "6.0.1", "2026-09-20", SLUG)
except CheckError as exc:
    assert "notice paragraph" in str(exc)
else:
    raise AssertionError("malformed notice paragraph was not detected")

try:
    rollover_changelog(CHANGELOG_EMPTY_UNRELEASED, "6.0.2", "6.0.0", "2026-09-20", SLUG)
except CheckError as exc:
    assert "previous version" in str(exc)
else:
    raise AssertionError("previous-version mismatch was not detected")

try:
    rollover_changelog(CHANGELOG_EMPTY_UNRELEASED, "not-a-version", "6.0.1", "2026-09-20", SLUG)
except CheckError:
    pass
else:
    raise AssertionError("invalid new_version was not rejected")

try:
    rollover_changelog(CHANGELOG_EMPTY_UNRELEASED, "6.0.2", "6.0.1", "20-09-2026", SLUG)
except CheckError as exc:
    assert "invalid date" in str(exc)
else:
    raise AssertionError("invalid date format was not rejected")

print("PASS: rollover_changelog fails closed on malformed markers, version/date input")

# Second invocation against the *already-rolled-over* text, targeting the
# same version again (now correctly matching the new Unreleased compare
# link's previous version), must still fail closed on the duplicate-section
# guard -- proving the operation is not silently re-appliable.
try:
    rollover_changelog(new_text, "6.0.2", "6.0.2", "2026-09-21", SLUG)
except CheckError as exc:
    assert "already contains a section for 6.0.2" in str(exc)
else:
    raise AssertionError("re-running rollover for an already-rolled-over version did not fail")

print("PASS: rollover_changelog second invocation for the same version fails closed (safe non-idempotent)")


# ---------------------------------------------------------------------------
# build_provenance
# ---------------------------------------------------------------------------

COMPAT = {
    "streamnzb_version": "6.1.0",
    "streamnzb_sha": "2ff93449e59a6597f25fd008e3440920772578c3",
    "jhin_version": "0.7.1",
}
COUNTS = {"samsung": 151, "neutral": 150, "core": 129, "presentation": 21, "device": 1, "defines": 62}

provenance = build_provenance("6.0.2", "6.0.1", "8fea02c16c204988af3a1acf08dd303ada9f0414", "2026-09-20", COMPAT, COUNTS)
assert provenance["version"] == "6.0.2"
assert provenance["previous_version"] == "6.0.1"
assert provenance["prepared_from_sha"] == "8fea02c16c204988af3a1acf08dd303ada9f0414"
assert provenance["prepared_at_date"] == "2026-09-20"
assert provenance["compatibility"] == COMPAT
assert provenance["expected_counts"] == COUNTS
assert provenance["schema_version"] == 1

print("PASS: build_provenance produces the expected schema")

try:
    build_provenance("6.0.2", "6.0.1", "not-a-sha", "2026-09-20", COMPAT, COUNTS)
except CheckError as exc:
    assert "prepared_from_sha" in str(exc)
else:
    raise AssertionError("invalid prepared_from_sha was not rejected")

try:
    build_provenance("6.0.2", "6.0.1", "8fea02c16c204988af3a1acf08dd303ada9f0414", "09-20-2026", COMPAT, COUNTS)
except CheckError as exc:
    assert "prepared_at_date" in str(exc)
else:
    raise AssertionError("invalid prepared_at_date was not rejected")

print("PASS: build_provenance rejects invalid sha/date")


# ---------------------------------------------------------------------------
# generate_release_note
# ---------------------------------------------------------------------------

note = generate_release_note(
    "6.0.2", "6.0.1", "### Bug Fixes\n\n- fixed a thing.", COMPAT, COUNTS, SLUG
)
assert "DraCuLa StreamNZB Template 6.0.2" in note
assert "### Bug Fixes\n\n- fixed a thing." in note
assert "StreamNZB: 6.1.0" in note
assert "`2ff93449e59a6597f25fd008e3440920772578c3`" in note
assert "Jhin: 0.7.1" in note
assert "Samsung QN90A: **151**" in note
assert "Hardware Neutral: **150**" in note
assert "Shared Core: **129**" in note
assert "Presentation: **21**" in note
assert "Samsung device-specific: **1**" in note
assert "Define Library: **62**" in note
assert f"https://github.com/{SLUG}/compare/6.0.1...6.0.2" in note

# Deterministic: identical inputs produce byte-identical output.
note_again = generate_release_note(
    "6.0.2", "6.0.1", "### Bug Fixes\n\n- fixed a thing.", COMPAT, COUNTS, SLUG
)
assert note == note_again

print("PASS: generate_release_note includes version/compat/counts/compare-link, deterministic")

empty_note = generate_release_note("6.0.2", "6.0.1", "", COMPAT, COUNTS, SLUG)
assert "_No changes recorded._" in empty_note

print("PASS: generate_release_note handles an empty changelog body without fabricating content")

print("PASS: prepare_release_housekeeping tests")
