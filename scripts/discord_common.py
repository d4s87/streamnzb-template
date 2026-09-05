#!/usr/bin/env python3
"""Shared helpers for building and writing Discord notification payloads.

Used by formatter_discord.py, release_discord.py, and vidhin_discord.py so
the Discord payload envelope, message-length limit, and truncation helper
are defined once instead of being reimplemented per notification type.
"""

import json
from pathlib import Path


MAX_MESSAGE_LENGTH = 2000


def truncate_with_ellipsis(value, max_length):
    """Truncate value to at most max_length characters, ending in an ellipsis."""
    if len(value) <= max_length:
        return value

    return value[:max_length - 1].rstrip() + "…"


def wrap_discord_content(message):
    """Wrap plain-text message content in the shared Discord payload envelope."""
    if len(message) > MAX_MESSAGE_LENGTH:
        raise ValueError(
            f"Discord message exceeds {MAX_MESSAGE_LENGTH} characters"
        )

    return {
        "content": message,
        "allowed_mentions": {
            "parse": [],
        },
    }


def write_discord_payload(path, payload):
    """Write a Discord payload as JSON to path; return its message content."""
    Path(path).write_text(
        json.dumps(payload, ensure_ascii=False),
        encoding="utf-8",
    )

    return payload["content"]
