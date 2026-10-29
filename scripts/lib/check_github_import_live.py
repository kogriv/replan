"""Prepare and summarize the disposable GitHub live-import check."""

import argparse
import copy
import json
import pathlib

import yaml


def add_profile(project_file, repository, token_env):
    path = pathlib.Path(project_file)
    with path.open(encoding="utf-8") as stream:
        data = yaml.safe_load(stream)
    profiles = data["project"]["source_profiles"]
    demo = next(profile for profile in profiles if profile["id"] == "github.demo")
    live = copy.deepcopy(demo)
    live["id"] = "github.live-check"
    live["repository"] = repository
    live["token_env"] = token_env
    profiles.append(live)
    with path.open("w", encoding="utf-8") as stream:
        yaml.safe_dump(data, stream, sort_keys=False)


def load_json(path):
    with open(path, encoding="utf-8") as stream:
        return json.load(stream)


def snapshot_id(import_file):
    return load_json(import_file)["result"]["snapshot"]["id"]


def summarize(import_file, accept_directory):
    source = load_json(import_file)
    accept_dir = pathlib.Path(accept_directory)
    accepts = {path.stem: load_json(path) for path in sorted(accept_dir.glob("*.json"))}
    time_entry_codes = [item["code"] for item in accepts["time_entries"]["errors"]]
    if "import_accept_time_entries_unsupported_source" not in time_entry_codes:
        raise ValueError("GitHub time-entry acceptance did not return the expected diagnostic")
    result = source["result"]
    warnings = source.get("warnings", [])
    return {
        "ok": True,
        "command": "check-github-import-live",
        "mode": "read_only",
        "counts": {
            "issues": len(result["task_candidates"]),
            "resource_candidates": len(result["resource_candidates"]),
            "resource_demand_candidates": len(result.get("resource_demand_candidates", [])),
            "dependency_candidates": len(result.get("dependency_candidates", [])),
            "accept_dry_runs": len(accepts),
        },
        "snapshot_id": result["snapshot"]["id"],
        "rate_limit_remaining": result.get("rate_limit_remaining"),
        "warnings": warnings,
        "errors": [],
        "warning_codes": [item["code"] for item in warnings],
        "error_codes": [],
    }


def main():
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)
    setup = subparsers.add_parser("setup")
    setup.add_argument("project_file")
    setup.add_argument("repository")
    setup.add_argument("token_env")
    identify = subparsers.add_parser("snapshot-id")
    identify.add_argument("import_file")
    report = subparsers.add_parser("report")
    report.add_argument("import_file")
    report.add_argument("accept_directory")
    args = parser.parse_args()
    if args.command == "setup":
        add_profile(args.project_file, args.repository, args.token_env)
    elif args.command == "snapshot-id":
        print(snapshot_id(args.import_file))
    else:
        print(json.dumps(summarize(args.import_file, args.accept_directory), sort_keys=True))


if __name__ == "__main__":
    main()
