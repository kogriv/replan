#!/usr/bin/env python3
"""Check command references in the README of an unpacked agent kit."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


NAME = re.compile(r"[a-z0-9][a-z0-9-]*")
REPLAN_REFERENCE = re.compile(
    r"bin/replan\s+([A-Za-z0-9<>.\\|_-]+)(?:\s+([A-Za-z0-9<>.\\|_-]+))?"
)
HELPER_REFERENCE = re.compile(r"scripts/([A-Za-z0-9][A-Za-z0-9._-]*\.sh)")
LIB_REFERENCE = re.compile(r"scripts/lib/([A-Za-z0-9][A-Za-z0-9._/-]*(?:\.py|\.sh))")


def alternatives(token: str) -> list[str]:
    values = token.replace(r"\|", "|").split("|")
    if values and all(NAME.fullmatch(value) for value in values):
        return values
    return []


def accepted_commands(help_text: str) -> tuple[set[str], set[tuple[str, str]]]:
    commands: set[str] = set()
    subcommands: set[tuple[str, str]] = set()
    for line in help_text.splitlines():
        match = re.match(r"\s*replan\s+(\S+)(?:\s+(\S+))?", line)
        if not match:
            continue
        parents = alternatives(match.group(1))
        children = alternatives(match.group(2) or "")
        commands.update(parents)
        subcommands.update((parent, child) for parent in parents for child in children)
    return commands, subcommands


def check_commands(readme: Path, help_path: Path) -> list[str]:
    commands, subcommands = accepted_commands(help_path.read_text())
    issues: list[str] = []
    for line_number, line in enumerate(readme.read_text().splitlines(), 1):
        for match in REPLAN_REFERENCE.finditer(line):
            parents = alternatives(match.group(1))
            if not parents:
                continue
            children = alternatives(match.group(2) or "")
            for parent in parents:
                if parent not in commands:
                    issues.append(
                        f"missing command bin/replan {parent} at README.md:{line_number}"
                    )
                    continue
                for child in children:
                    if (parent, child) not in subcommands:
                        issues.append(
                            f"missing command bin/replan {parent} {child} "
                            f"at README.md:{line_number}"
                        )
    return issues


def check_helpers(readme: Path, kit: Path) -> list[str]:
    issues: list[str] = []
    for line_number, line in enumerate(readme.read_text().splitlines(), 1):
        for match in HELPER_REFERENCE.finditer(line):
            relative_path = Path("scripts") / match.group(1)
            helper = kit / relative_path
            if not helper.is_file() or not helper.stat().st_mode & 0o111:
                issues.append(
                    f"missing or non-executable helper {relative_path} "
                    f"at README.md:{line_number}"
                )
    return issues


def check_lib_references(readme: Path) -> list[str]:
    issues: list[str] = []
    for line_number, line in enumerate(readme.read_text().splitlines(), 1):
        for match in LIB_REFERENCE.finditer(line):
            issues.append(
                f"direct library command scripts/lib/{match.group(1)} "
                f"at README.md:{line_number}"
            )
    return issues


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("check", choices=("commands", "helpers", "lib"))
    parser.add_argument("readme", type=Path)
    parser.add_argument("--help-output", type=Path)
    parser.add_argument("--kit", type=Path)
    args = parser.parse_args()

    if args.check == "commands":
        if args.help_output is None:
            parser.error("commands requires --help-output")
        issues = check_commands(args.readme, args.help_output)
    elif args.check == "helpers":
        if args.kit is None:
            parser.error("helpers requires --kit")
        issues = check_helpers(args.readme, args.kit)
    else:
        issues = check_lib_references(args.readme)

    if issues:
        print("; ".join(issues))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
