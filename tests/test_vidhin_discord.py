#!/usr/bin/env python3

import importlib.util
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

spec = importlib.util.spec_from_file_location(
    "vidhin_discord",
    ROOT / "scripts/vidhin_discord.py",
)

module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def document(defines, generated_at="2026-01-01T00:00:00+00:00"):
    return {
        "schema_version": 3,
        "generated_at_utc": generated_at,
        "defines": defines,
    }


base_defines = {
    "Alpha Groups": {
        "scope": "movie",
        "field": "group",
        "tokens": ["Alpha"],
    },
    "Beta Groups": {
        "scope": "series",
        "field": "group",
        "tokens": ["Beta"],
    },
}

base = document(base_defines)

# Identical semantic baseline.
result = module.analyze(
    base,
    document(json.loads(json.dumps(base_defines))),
    "library-v1",
    "library-v1",
)

assert result["notify"] is False
assert result["added"] == []
assert result["changed"] == []
assert result["removed"] == []
assert result["library_changed"] is False

# Metadata-only JSON change must not notify.
metadata_only = document(
    json.loads(json.dumps(base_defines)),
    generated_at="2099-01-01T00:00:00+00:00",
)

result = module.analyze(
    base,
    metadata_only,
    "library-v1",
    "library-v1",
)

assert result["notify"] is False

# Changed Define.
changed_defines = json.loads(json.dumps(base_defines))
changed_defines["Alpha Groups"]["tokens"].append("Gamma")

result = module.analyze(
    base,
    document(changed_defines),
    "library-v1",
    "library-v2",
)

assert result["notify"] is True
assert result["changed"] == ["Alpha Groups"]
assert result["added"] == []
assert result["removed"] == []
assert result["library_changed"] is True

# Added Define.
added_defines = json.loads(json.dumps(base_defines))
added_defines["Gamma Groups"] = {
    "scope": "movie",
    "field": "group",
    "tokens": ["Gamma"],
}

result = module.analyze(
    base,
    document(added_defines),
    "library-v1",
    "library-v2",
)

assert result["notify"] is True
assert result["added"] == ["Gamma Groups"]

# Removed Define.
removed_defines = json.loads(json.dumps(base_defines))
del removed_defines["Beta Groups"]

result = module.analyze(
    base,
    document(removed_defines),
    "library-v1",
    "library-v2",
)

assert result["notify"] is True
assert result["removed"] == ["Beta Groups"]

# Published-library-only change must notify even when the mapped
# classification JSON is semantically identical.
result = module.analyze(
    base,
    document(json.loads(json.dumps(base_defines))),
    "library-v1",
    "library-v2",
)

assert result["notify"] is True
assert result["added"] == []
assert result["changed"] == []
assert result["removed"] == []
assert result["library_changed"] is True

message = module.build_message(
    result,
    repository="d4s87/streamnzb-template",
    server_url="https://github.com",
    commit_sha="0123456789abcdef",
)

assert "DraCuLa Define Library updated" in message
assert "published StreamNZB Define Library changed" in message
assert "Vidhin classification changes:" not in message
assert "Define Libraries" in message
assert "Refresh" in message
assert (
    "https://github.com/d4s87/streamnzb-template/"
    "commit/0123456789abcdef"
    in message
)
assert len(message) <= 2000

# Named changes are included in the user-facing message.
named = module.analyze(
    base,
    document(changed_defines),
    "library-v1",
    "library-v2",
)

message = module.build_message(
    named,
    repository="d4s87/streamnzb-template",
    server_url="https://github.com/",
    commit_sha="fedcba9876543210",
)

assert "🔄 Alpha Groups" in message
assert "**Vidhin classification changes:**" in message
assert "Refresh the **Define Library first**" in message

payload = {
    "content": message,
    "allowed_mentions": {
        "parse": [],
    },
}

assert payload["allowed_mentions"] == {"parse": []}

print("PASS: Vidhin Discord notification tests")
