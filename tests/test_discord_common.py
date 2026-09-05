#!/usr/bin/env python3
import importlib.util
import json
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

spec = importlib.util.spec_from_file_location(
    "discord_common",
    ROOT / "scripts/discord_common.py",
)

module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


# truncate_with_ellipsis leaves short/exact-length values untouched.
assert module.truncate_with_ellipsis("short", 10) == "short"
assert module.truncate_with_ellipsis("exactlyten", 10) == "exactlyten"

# Values over the limit are cut and end in an ellipsis, never exceeding
# max_length characters.
truncated = module.truncate_with_ellipsis("x" * 30, 10)
assert len(truncated) == 10
assert truncated.endswith("…")

# Trailing whitespace left by truncation is stripped before the ellipsis.
padded = module.truncate_with_ellipsis("123456   890", 9)
assert padded == "123456…"


# wrap_discord_content produces the shared payload envelope.
payload = module.wrap_discord_content("hello world")
assert payload == {
    "content": "hello world",
    "allowed_mentions": {"parse": []},
}

# Content at exactly the limit is accepted.
module.wrap_discord_content("x" * module.MAX_MESSAGE_LENGTH)

# Content over the limit fails closed.
try:
    module.wrap_discord_content("x" * (module.MAX_MESSAGE_LENGTH + 1))
except ValueError as exc:
    assert str(module.MAX_MESSAGE_LENGTH) in str(exc)
else:
    raise AssertionError(
        "over-limit Discord content was not rejected"
    )


# write_discord_payload writes valid JSON and returns the message content.
with tempfile.TemporaryDirectory() as tmpdir:
    output_path = Path(tmpdir) / "payload.json"

    returned = module.write_discord_payload(
        output_path,
        payload,
    )

    assert returned == "hello world"

    written = json.loads(output_path.read_text(encoding="utf-8"))
    assert written == payload

print("PASS: shared Discord payload helper tests")
