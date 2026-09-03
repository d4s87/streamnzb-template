#!/usr/bin/env python3

import argparse
import base64
import gzip
import io
import json
import os
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

RULES_PATH = ROOT / "profiles" / "rules.json"
VARIANTS_PATH = ROOT / "profiles" / "variants.json"

PROFILE_PREFIX = "SNZBP1:"
EXPECTED_SOURCE_SCHEMA = 1
EXPECTED_STREAMNZB_SCHEMA = 1

EXPECTED_OWNERS = {
    "core",
    "presentation",
    "device:samsung-qn90a",
}

EXPECTED_DEVICE_RULES = [
    "DV without HDR fallback",
    "Reduce Atmos",
    "Reduce TrueHD bonus",
    "Reduce DTS Lossless bonus",
]

EXPECTED_PRESENTATION_RULES = {
    "10bit",
    "B-Global",
    "Bilibili",
    "HIDIVE",
}


def load_json(path: Path):
    return json.loads(
        path.read_text(encoding="utf-8")
    )


def atomic_write(path: Path, text: str):
    path.parent.mkdir(parents=True, exist_ok=True)

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

        os.replace(temporary, path)
    except Exception:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def encode_payload(payload: dict) -> str:
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
        buffer.getvalue()
    ).decode("ascii").rstrip("=")

    return PROFILE_PREFIX + encoded + "\n"


def decode_share_code(text: str) -> dict:
    code = text.strip()

    if "\n" in code or "\r" in code:
        raise ValueError(
            "profile artifact must contain exactly one share code"
        )

    if not code.startswith(PROFILE_PREFIX):
        raise ValueError(
            f"profile artifact does not start with {PROFILE_PREFIX!r}"
        )

    encoded = code[len(PROFILE_PREFIX):]

    if not encoded:
        raise ValueError("profile share code has no payload")

    encoded += "=" * (-len(encoded) % 4)

    compressed = base64.urlsafe_b64decode(encoded)
    raw = gzip.decompress(compressed)
    payload = json.loads(raw.decode("utf-8"))

    if not isinstance(payload, dict):
        raise ValueError(
            "decoded profile payload must be an object"
        )

    return payload


def validate_registry(payload: dict):
    if set(payload) != {
        "schema_version",
        "streamnzb_profile",
        "rules",
    }:
        raise ValueError(
            "rules source keys differ from expected schema"
        )

    if payload["schema_version"] != EXPECTED_SOURCE_SCHEMA:
        raise ValueError(
            "unsupported rules source schema"
        )

    if payload["streamnzb_profile"] != EXPECTED_STREAMNZB_SCHEMA:
        raise ValueError(
            "unsupported StreamNZB profile schema"
        )

    entries = payload["rules"]

    if not isinstance(entries, list):
        raise ValueError("rules source must contain a rules array")

    if len(entries) != 113:
        raise ValueError(
            f"expected 113 source rules, found {len(entries)}"
        )

    names = []
    owner_counts = {}

    for index, entry in enumerate(entries, start=1):
        if not isinstance(entry, dict):
            raise ValueError(
                f"source entry #{index} is not an object"
            )

        if set(entry) != {"owner", "rule"}:
            raise ValueError(
                f"source entry #{index} has unexpected keys"
            )

        owner = entry["owner"]
        rule = entry["rule"]

        if owner not in EXPECTED_OWNERS:
            raise ValueError(
                f"source entry #{index} has unknown owner {owner!r}"
            )

        if not isinstance(rule, dict):
            raise ValueError(
                f"source entry #{index} rule is not an object"
            )

        name = rule.get("name")

        if not isinstance(name, str) or not name.strip():
            raise ValueError(
                f"source entry #{index} has invalid rule name"
            )

        names.append(name)
        owner_counts[owner] = owner_counts.get(owner, 0) + 1

    if len(set(names)) != len(names):
        raise ValueError("source contains duplicate rule names")

    expected_counts = {
        "core": 105,
        "presentation": 4,
        "device:samsung-qn90a": 4,
    }

    if owner_counts != expected_counts:
        raise ValueError(
            f"unexpected ownership counts: {owner_counts!r}"
        )

    device_names = [
        entry["rule"]["name"]
        for entry in entries
        if entry["owner"] == "device:samsung-qn90a"
    ]

    if device_names != EXPECTED_DEVICE_RULES:
        raise ValueError(
            "Samsung device-rule membership/order differs"
        )

    presentation_names = {
        entry["rule"]["name"]
        for entry in entries
        if entry["owner"] == "presentation"
    }

    if presentation_names != EXPECTED_PRESENTATION_RULES:
        raise ValueError(
            "presentation-rule membership differs"
        )

    reject_3d = [
        entry
        for entry in entries
        if entry["rule"]["name"] == "Reject 3D"
    ]

    if len(reject_3d) != 1:
        raise ValueError(
            f"expected one Reject 3D rule, found {len(reject_3d)}"
        )

    if reject_3d[0]["owner"] != "core":
        raise ValueError(
            "Reject 3D must remain owned by core"
        )

    return entries


def validate_variants(payload: dict):
    if set(payload) != {"schema_version", "variants"}:
        raise ValueError(
            "variants source keys differ from expected schema"
        )

    if payload["schema_version"] != EXPECTED_SOURCE_SCHEMA:
        raise ValueError(
            "unsupported variants source schema"
        )

    variants = payload["variants"]

    if not isinstance(variants, list) or len(variants) != 2:
        raise ValueError(
            "expected exactly two profile variants"
        )

    expected = {
        "profile.txt": {
            "name": "Samsung QN90A",
            "preset": "4k",
            "owners": [
                "core",
                "presentation",
                "device:samsung-qn90a",
            ],
            "expected_rules": 113,
        },
        "profile-neutral.txt": {
            "name": "DraCuLa Neutral",
            "preset": "4k",
            "owners": [
                "core",
                "presentation",
            ],
            "expected_rules": 109,
        },
    }

    seen = set()

    for index, variant in enumerate(variants, start=1):
        if not isinstance(variant, dict):
            raise ValueError(
                f"variant #{index} is not an object"
            )

        if set(variant) != {
            "artifact",
            "name",
            "preset",
            "owners",
            "expected_rules",
        }:
            raise ValueError(
                f"variant #{index} has unexpected keys"
            )

        artifact = variant["artifact"]

        if artifact not in expected:
            raise ValueError(
                f"unexpected variant artifact {artifact!r}"
            )

        if artifact in seen:
            raise ValueError(
                f"duplicate variant artifact {artifact!r}"
            )

        seen.add(artifact)

        wanted = expected[artifact]

        for key, value in wanted.items():
            if variant[key] != value:
                raise ValueError(
                    f"{artifact} {key} differs: "
                    f"{variant[key]!r} != {value!r}"
                )

    if seen != set(expected):
        raise ValueError(
            "required variant artifact missing"
        )

    return variants


def build_variant(
    *,
    entries,
    variant,
    streamnzb_profile,
):
    owners = set(variant["owners"])

    rules = [
        entry["rule"]
        for entry in entries
        if entry["owner"] in owners
    ]

    if len(rules) != variant["expected_rules"]:
        raise ValueError(
            f"{variant['artifact']} expected "
            f"{variant['expected_rules']} rules, found {len(rules)}"
        )

    payload = {
        "name": variant["name"],
        "preset": variant["preset"],
        "rules": rules,
        "streamnzb_profile": streamnzb_profile,
    }

    return payload


def validate_cross_variant(
    samsung: dict,
    neutral: dict,
):
    samsung_rules = samsung["rules"]
    neutral_rules = neutral["rules"]

    samsung_names = [
        rule["name"]
        for rule in samsung_rules
    ]
    neutral_names = [
        rule["name"]
        for rule in neutral_rules
    ]

    device_set = set(EXPECTED_DEVICE_RULES)

    expected_neutral_names = [
        name
        for name in samsung_names
        if name not in device_set
    ]

    if neutral_names != expected_neutral_names:
        raise ValueError(
            "neutral rule order/content membership differs from "
            "Samsung-minus-device expectation"
        )

    samsung_by_name = {
        rule["name"]: rule
        for rule in samsung_rules
    }

    neutral_by_name = {
        rule["name"]: rule
        for rule in neutral_rules
    }

    for name in neutral_names:
        if neutral_by_name[name] != samsung_by_name[name]:
            raise ValueError(
                f"shared rule {name!r} differs between variants"
            )

    if "Reject 3D" not in neutral_by_name:
        raise ValueError(
            "Reject 3D must be present in neutral profile"
        )

    leaked = device_set & set(neutral_names)

    if leaked:
        raise ValueError(
            "device rule leaked into neutral profile: "
            + ", ".join(sorted(leaked))
        )


def build_all():
    rules_source = load_json(RULES_PATH)
    variants_source = load_json(VARIANTS_PATH)

    entries = validate_registry(rules_source)
    variants = validate_variants(variants_source)

    payloads = {}

    for variant in variants:
        payloads[variant["artifact"]] = build_variant(
            entries=entries,
            variant=variant,
            streamnzb_profile=rules_source["streamnzb_profile"],
        )

    samsung = payloads["profile.txt"]
    neutral = payloads["profile-neutral.txt"]

    validate_cross_variant(
        samsung,
        neutral,
    )

    encoded = {
        artifact: encode_payload(payload)
        for artifact, payload in payloads.items()
    }

    # Decode every generated artifact before any write.
    for artifact, text in encoded.items():
        rebuilt = decode_share_code(text)

        if rebuilt != payloads[artifact]:
            raise ValueError(
                f"{artifact} failed encode/decode round-trip"
            )

    return payloads, encoded


def command_check(_args):
    payloads, encoded = build_all()

    current_samsung = (ROOT / "profile.txt").read_text(
        encoding="utf-8"
    )

    if encoded["profile.txt"] != current_samsung:
        raise SystemExit(
            "ERROR: generated profile.txt differs from current "
            "Samsung behavior-preserving artifact"
        )

    neutral_path = ROOT / "profile-neutral.txt"

    if neutral_path.exists():
        current_neutral = neutral_path.read_text(
            encoding="utf-8"
        )

        if encoded["profile-neutral.txt"] != current_neutral:
            raise SystemExit(
                "ERROR: profile-neutral.txt is not up to date"
            )

    print("PASS: source registry and variants validated")
    print(
        f"PASS: profile.txt canonical output unchanged "
        f"({len(payloads['profile.txt']['rules'])} rules)"
    )

    if neutral_path.exists():
        print(
            "PASS: profile-neutral.txt is current "
            f"({len(payloads['profile-neutral.txt']['rules'])} rules)"
        )
    else:
        print(
            "PASS: neutral variant validates "
            f"({len(payloads['profile-neutral.txt']['rules'])} rules; "
            "artifact not written yet)"
        )

    print("PASS: Reject 3D is present in both variants")
    print("PASS: exactly four Samsung-only rules")


def command_build(_args):
    payloads, encoded = build_all()

    # Post-V5 policy changes are intentional canonical-source changes.
    # build_all() validates source/schema/ownership/variants and performs
    # encode/decode round-trip validation before any artifact write.

    # All validation above completed before any write.
    atomic_write(
        ROOT / "profile.txt",
        encoded["profile.txt"],
    )

    atomic_write(
        ROOT / "profile-neutral.txt",
        encoded["profile-neutral.txt"],
    )

    print(
        "PASS: profile.txt reproduced byte-for-byte "
        f"({len(payloads['profile.txt']['rules'])} rules)"
    )
    print(
        "PASS: profile-neutral.txt generated "
        f"({len(payloads['profile-neutral.txt']['rules'])} rules)"
    )
    print("PASS: Reject 3D present in both profiles")
    print("PASS: neutral excludes exactly four Samsung device rules")


def build_parser():
    parser = argparse.ArgumentParser(
        description=(
            "Build DraCuLa StreamNZB profile variants "
            "from the canonical ordered rule registry."
        )
    )

    subparsers = parser.add_subparsers(
        dest="command",
        required=True,
    )

    check = subparsers.add_parser(
        "check",
        help="validate sources and generated outputs without writing",
    )
    check.set_defaults(func=command_check)

    build = subparsers.add_parser(
        "build",
        help="validate and write generated profile artifacts",
    )
    build.set_defaults(func=command_build)

    return parser


def main():
    args = build_parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
