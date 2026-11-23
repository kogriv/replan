"""Generate the Go lookup from the shared OpenProject status map."""

import json
import os


HERE = os.path.dirname(__file__)
SOURCE = os.path.abspath(
    os.path.join(HERE, "..", "..", "..", "scripts", "lib", "openproject_status_map.json")
)
OUTPUT = os.path.abspath(os.path.join(HERE, "..", "openproject_status_map_generated.go"))


def normalize(value):
    return "".join(character for character in value.lower() if character.isalnum())


def main():
    with open(SOURCE, encoding="utf-8") as handle:
        pairs = json.load(handle)
    entries = []
    for pair in pairs:
        names = [pair["openproject_status"], *(pair.get("aliases") or [])]
        entries.extend((normalize(name), pair["tracking_status"]) for name in names)
    width = max(len(json.dumps(name)) for name, _ in entries)
    lines = [
        "// Code generated from scripts/lib/openproject_status_map.json; DO NOT EDIT.\n",
        "\npackage app\n\n",
        "var openProjectStatusByNormalizedName = map[string]string{\n",
    ]
    lines.extend(
        f"\t{json.dumps(name):<{width}}: {json.dumps(status)},\n"
        for name, status in entries
    )
    lines.append("}\n")
    with open(OUTPUT, "w", encoding="utf-8", newline="\n") as handle:
        handle.writelines(lines)


if __name__ == "__main__":
    main()
