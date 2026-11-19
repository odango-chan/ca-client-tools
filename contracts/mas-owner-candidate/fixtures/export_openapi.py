"""Export the WTLab MAS Skill projections as an OpenAPI component catalog."""

import argparse
import json
from pathlib import Path

import yaml


def export(candidate: Path) -> dict:
    components = {}
    for path in sorted((candidate / "schemas").glob("*.schema.json")):
        name = path.name.removesuffix(".schema.json")
        key = "".join(part.capitalize() for part in name.replace(".", "-").split("-"))
        if key in components:
            raise ValueError(f"duplicate OpenAPI component {key}")
        schema = json.loads(path.read_text(encoding="utf-8"))
        components[key] = schema
    return {
        "openapi": "3.1.0",
        "info": {
            "title": "OpenAMSM MAS Skill Schema Candidate",
            "version": candidate.name.removeprefix("v"),
            "description": (
                "Skill-facing schema projections for the fixed MAS establishment profile. "
                "WTLab identifies the OpenAMSM projections. "
                "The OpenAMSM HTTP API is documented separately."
            ),
        },
        "x-schema-id-namespace": "wtlab",
        "components": {"schemas": components},
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path)
    parser.add_argument("--candidate", type=Path, default=Path(__file__).resolve().parents[1] / "v0.2.0")
    args = parser.parse_args()
    args.output.write_text(yaml.safe_dump(export(args.candidate), sort_keys=False, allow_unicode=True), encoding="utf-8")
