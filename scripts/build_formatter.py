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

    if not code.startswith(PREFIX):
        raise ValueError(
            f"{path} does not start with {PREFIX!r}"
        )

    encoded = code[len(PREFIX):]
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


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Build formatter.txt from formatter.source.json "
            "or verify semantic synchronization."
        )
    )

    parser.add_argument(
        "--check",
        action="store_true",
        help=(
            "verify formatter.source.json and formatter.txt "
            "are semantically synchronized"
        ),
    )

    args = parser.parse_args()

    root = Path(__file__).resolve().parents[1]

    source_path = (
        root / "tests/streamnzb_compat/formatter.source.json"
    )

    output_path = (
        root / "formatter.txt"
    )

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
                "ERROR: could not decode existing formatter.txt: "
                f"{exc}",
                file=sys.stderr,
            )
            return 1

    synchronized = (
        current == source
    )

    if args.check:
        if not synchronized:
            print(
                "ERROR: formatter.source.json and formatter.txt "
                "are not semantically synchronized.",
                file=sys.stderr,
            )
            return 1

        print(
            "PASS: formatter.source.json and formatter.txt "
            "are semantically synchronized."
        )

        return 0

    if synchronized:
        print(
            "No formatter.txt update required; source and "
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
            f"ERROR: could not write formatter.txt: {exc}",
            file=sys.stderr,
        )
        return 1

    try:
        rebuilt = decode_share_code(
            output_path,
        )
    except Exception as exc:
        print(
            "ERROR: generated formatter.txt could not "
            f"be decoded: {exc}",
            file=sys.stderr,
        )
        return 1

    if rebuilt != source:
        print(
            "ERROR: generated formatter.txt failed "
            "semantic round-trip.",
            file=sys.stderr,
        )
        return 1

    print(
        "Generated formatter.txt from formatter.source.json."
    )

    print(
        "PASS: generated formatter semantic "
        "round-trip verified."
    )

    return 0


if __name__ == "__main__":
    sys.exit(main())
