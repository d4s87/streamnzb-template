#!/usr/bin/env python3

import importlib.util


from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

spec = importlib.util.spec_from_file_location(
    "release_discord",
    ROOT / "scripts/release_discord.py",
)

module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def event(
    *,
    action="published",
    name="DraCuLa StreamNZB Template V5.1",
    tag_name="v5.1",
    html_url=(
        "https://github.com/d4s87/streamnzb-template/"
        "releases/tag/v5.1"
    ),
    draft=False,
    prerelease=False,
):
    return {
        "action": action,
        "release": {
            "name": name,
            "tag_name": tag_name,
            "html_url": html_url,
            "draft": draft,
            "prerelease": prerelease,
        },
    }


# Stable release.
payload = module.build_payload(event())

assert payload["allowed_mentions"] == {
    "parse": [],
}

message = payload["content"]

assert "release published" in message
assert "DraCuLa StreamNZB Template V5.1" in message
assert "`v5.1`" in message
assert "Define Library first" in message
assert (
    "https://github.com/d4s87/streamnzb-template/"
    "releases/tag/v5.1"
    in message
)
assert len(message) <= 2000


# Pre-release uses an explicit pre-release heading.
payload = module.build_payload(
    event(
        name="DraCuLa StreamNZB Template V5.2 RC1",
        tag_name="v5.2-rc1",
        prerelease=True,
    )
)

assert "pre-release published" in payload["content"]


# Empty release name falls back to tag.
payload = module.build_payload(
    event(
        name="",
        tag_name="v6.0",
    )
)

assert "**v6.0** (`v6.0`)" in payload["content"]


# Release names are bounded so user-controlled release metadata cannot
# unexpectedly exceed Discord's message limit.
release = module.release_from_event(
    event(name="X" * 1000)
)

assert len(release["name"]) <= module.MAX_RELEASE_NAME_LENGTH
assert release["name"].endswith("…")


# Only published release events are valid input.
try:
    module.build_payload(
        event(action="created")
    )
except ValueError as exc:
    assert "published" in str(exc)
else:
    raise AssertionError(
        "non-published release event unexpectedly accepted"
    )


# Draft releases must never notify.
try:
    module.build_payload(
        event(draft=True)
    )
except ValueError as exc:
    assert "draft" in str(exc)
else:
    raise AssertionError(
        "draft release unexpectedly accepted"
    )


# Required release metadata must be present.
for field in ("tag_name", "html_url"):
    broken = event()
    broken["release"][field] = ""

    try:
        module.build_payload(broken)
    except ValueError as exc:
        assert field in str(exc)
    else:
        raise AssertionError(
            f"missing {field} unexpectedly accepted"
        )


print("PASS: release Discord notification tests")
