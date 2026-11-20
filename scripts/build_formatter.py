#!/usr/bin/env python3

from pathlib import Path
import argparse
import base64
import gzip
import io
import json
import os
import sys
import tempfile

PREFIX = "SNZBF1:"
EXPECTED_SCHEMA = 1

# Every published formatter: (readable source, generated SNZBF1 share code).
# `--all` builds or checks each pair, so release tooling covers all of them.
FORMATTERS = (
    ("tests/streamnzb_compat/formatter.source.json", "formatter.txt"),
    ("tests/streamnzb_compat/formatter-debug.source.json", "formatter-debug.txt"),
    ("tests/streamnzb_compat/formatter-jellyfin.source.json", "formatter-jellyfin.txt"),
)

EXPECTED_KEYS = {
    "streamnzb_format_profile",
    "name",
    "result_name_template",
    "result_description_template",
}


def decode_share_code(path: Path):
    code = path.read_text(
        encoding="utf-8",
    ).strip()

    if "\n" in code or "\r" in code:
        raise ValueError(
            f"{path} must contain exactly one share code"
        )

    if not code.startswith(PREFIX):
        raise ValueError(
            f"{path} does not start with {PREFIX!r}"
        )

    encoded = code[len(PREFIX):]

    if not encoded:
        raise ValueError(f"{path} share code has no payload")

    encoded += "=" * (-len(encoded) % 4)

    compressed = base64.urlsafe_b64decode(
        encoded,
    )

    raw = gzip.decompress(
        compressed,
    )

    return json.loads(raw)


def load_source(path: Path):
    payload = json.loads(
        path.read_text(
            encoding="utf-8",
        )
    )

    if set(payload) != EXPECTED_KEYS:
        raise ValueError(
            "formatter source keys differ from expected schema: "
            f"{sorted(payload)}"
        )

    if payload["streamnzb_format_profile"] != EXPECTED_SCHEMA:
        raise ValueError(
            "unsupported formatter schema: "
            f"{payload['streamnzb_format_profile']}"
        )

    for key in (
        "name",
        "result_name_template",
        "result_description_template",
    ):
        if not isinstance(payload[key], str):
            raise ValueError(
                f"{key} must be a string"
            )

    return payload


def encode_payload(payload):
    raw = json.dumps(
        payload,
        ensure_ascii=False,
        separators=(",", ":"),
    ).encode("utf-8")

    buffer = io.BytesIO()

    with gzip.GzipFile(
        fileobj=buffer,
        mode="wb",
        compresslevel=9,
        mtime=0,
    ) as gz:
        gz.write(raw)

    encoded = base64.urlsafe_b64encode(
        buffer.getvalue(),
    ).decode("ascii").rstrip("=")

    return PREFIX + encoded + "\n"


def atomic_write(path: Path, text: str):
    fd, temporary = tempfile.mkstemp(
        dir=path.parent,
        prefix=f".{path.name}.",
        text=True,
    )

    try:
        with os.fdopen(
            fd,
            "w",
            encoding="utf-8",
        ) as handle:
            handle.write(text)

        os.replace(
            temporary,
            path,
        )

    except Exception:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def process(root: Path, source_rel: str, output_rel: str, check: bool) -> int:
    source_path = root / source_rel
    output_path = root / output_rel

    try:
        source = load_source(
            source_path,
        )
    except Exception as exc:
        print(
            f"ERROR: {exc}",
            file=sys.stderr,
        )
        return 1

    current = None

    if output_path.is_file():
        try:
            current = decode_share_code(
                output_path,
            )
        except Exception as exc:
            print(
                f"ERROR: could not decode existing {output_path.name}: "
                f"{exc}",
                file=sys.stderr,
            )
            return 1

    synchronized = (
        current == source
    )

    if check:
        if not synchronized:
            print(
                f"ERROR: {source_path} and {output_path} "
                "are not semantically synchronized.",
                file=sys.stderr,
            )
            return 1

        print(
            f"PASS: {source_path} and {output_path} "
            "are semantically synchronized."
        )

        return 0

    if synchronized:
        print(
            f"No {output_path.name} update required; source and "
            "published formatter are already equivalent."
        )
        return 0

    share_code = encode_payload(
        source,
    )

    try:
        atomic_write(
            output_path,
            share_code,
        )
    except Exception as exc:
        print(
            f"ERROR: could not write {output_path.name}: {exc}",
            file=sys.stderr,
        )
        return 1

    try:
        rebuilt = decode_share_code(
            output_path,
        )
    except Exception as exc:
        print(
            f"ERROR: generated {output_path.name} could not "
            f"be decoded: {exc}",
            file=sys.stderr,
        )
        return 1

    if rebuilt != source:
        print(
            f"ERROR: generated {output_path.name} failed "
            "semantic round-trip.",
            file=sys.stderr,
        )
        return 1

    print(
        f"Generated {output_path} from {source_path}."
    )

    print(
        "PASS: generated formatter semantic "
        "round-trip verified."
    )

    return 0


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Build a formatter share code from source JSON "
            "or verify semantic synchronization."
        )
    )

    parser.add_argument(
        "--check",
        action="store_true",
        help=(
            "verify source and published formatter "
            "are semantically synchronized"
        ),
    )

    parser.add_argument(
        "--all",
        action="store_true",
        help=(
            "process every published formatter (production, debug and "
            "Jellyfin); cannot be combined with --source/--output"
        ),
    )

    parser.add_argument(
        "--source",
        default=None,
        help=(
            "formatter source JSON path relative to repository root "
            f"(default: {FORMATTERS[0][0]})"
        ),
    )

    parser.add_argument(
        "--output",
        default=None,
        help=(
            "published SNZBF1 output path relative to repository root "
            f"(default: {FORMATTERS[0][1]})"
        ),
    )

    args = parser.parse_args()

    if args.all and (args.source or args.output):
        parser.error("--all cannot be combined with --source or --output")

    root = Path(__file__).resolve().parents[1]

    if args.all:
        pairs = FORMATTERS
    else:
        pairs = (
            (args.source or FORMATTERS[0][0], args.output or FORMATTERS[0][1]),
        )

    # Process every pair even after a failure so one run reports them all.
    results = [process(root, source, output, args.check) for source, output in pairs]
    return 1 if any(results) else 0


if __name__ == "__main__":
    sys.exit(main())
