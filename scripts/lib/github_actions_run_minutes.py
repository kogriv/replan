"""Read GitHub Actions jobs and calculate per-job billable minutes."""

import argparse
import json
import math
import os
import re
import shutil
import subprocess
from datetime import datetime

import helper_envelope


COMMAND = "github-actions-run-minutes"
helper_envelope.guard(COMMAND, "read_only")


class PublicFailure(RuntimeError):
    def __init__(self, code, message, exit_code):
        super().__init__(message)
        self.code = code
        self.exit_code = exit_code


def fail(code, message, exit_code, *, kind="", target_id=""):
    helper_envelope.emit({
        "ok": False,
        "command": COMMAND,
        "mode": "read_only" if exit_code != helper_envelope.EXIT["usage"] else "invalid",
        "errors": [helper_envelope.diagnostic(code, message, "error", kind, target_id)],
    }, exit_code)


class Parser(argparse.ArgumentParser):
    def error(self, message):
        fail("usage_error", f"{message}\n{self.format_usage().strip()}", helper_envelope.EXIT["usage"])


def parser():
    value = Parser(prog="github-actions-run-minutes.sh", allow_abbrev=False)
    value.add_argument("--run", action="append", required=True, dest="runs", metavar="ID")
    value.add_argument("--repo", metavar="OWNER/NAME")
    value.add_argument("--allow-incomplete", action="store_true")
    return value


def repository_from_remote():
    proc = subprocess.run(
        ["git", "remote", "get-url", "origin"], capture_output=True, text=True
    )
    if proc.returncode != 0:
        return ""
    remote = proc.stdout.strip().removesuffix("/").removesuffix(".git")
    match = re.search(r"(?:github\.com[/:])([^/\s:]+/[^/\s]+)$", remote)
    return match.group(1) if match else ""


def sanitize_stderr(text):
    cleaned = text.strip()
    secrets = {
        value
        for name, value in os.environ.items()
        if "TOKEN" in name.upper() and value
    }
    for secret in sorted(secrets, key=len, reverse=True):
        cleaned = cleaned.replace(secret, "[REDACTED]")
    return cleaned


def decode_json_stream(text):
    decoder = json.JSONDecoder()
    values = []
    index = 0
    while index < len(text):
        while index < len(text) and text[index].isspace():
            index += 1
        if index == len(text):
            break
        value, index = decoder.raw_decode(text, index)
        values.append(value)
    return values


def gh_api(endpoint, *, paginate=False):
    command = ["gh", "api", endpoint]
    if paginate:
        command.append("--paginate")
    proc = subprocess.run(command, capture_output=True, text=True)
    if proc.returncode != 0:
        detail = sanitize_stderr(proc.stderr) or f"gh exited {proc.returncode}"
        raise PublicFailure(
            "github_request_failed", f"GitHub request failed for {endpoint}: {detail}",
            helper_envelope.EXIT["external"],
        )
    try:
        values = decode_json_stream(proc.stdout)
    except (TypeError, ValueError, json.JSONDecodeError) as exc:
        raise PublicFailure(
            "github_request_failed", f"GitHub returned invalid JSON for {endpoint}: {exc}",
            helper_envelope.EXIT["external"],
        ) from exc
    if not values:
        raise PublicFailure(
            "github_request_failed", f"GitHub returned no JSON for {endpoint}",
            helper_envelope.EXIT["external"],
        )
    return values if paginate else values[0]


def parse_time(value):
    if not value:
        return None
    return datetime.fromisoformat(value[:-1] + "+00:00" if value.endswith("Z") else value)


def measured_job(job):
    started = parse_time(job.get("started_at"))
    completed = parse_time(job.get("completed_at"))
    skipped = job.get("conclusion") == "skipped"
    seconds = 0
    reason = ""
    if skipped:
        reason = "skipped"
    elif not started or not completed:
        reason = "timestamps_missing"
    else:
        seconds = max(0, (completed - started).total_seconds())
        if float(seconds).is_integer():
            seconds = int(seconds)
    return {
        "id": job.get("id"),
        "name": job.get("name", ""),
        "status": job.get("status", ""),
        "conclusion": job.get("conclusion"),
        "started_at": job.get("started_at"),
        "completed_at": job.get("completed_at"),
        "seconds": seconds,
        "billable_minutes": math.ceil(seconds / 60) if seconds else 0,
        "excluded_reason": reason,
    }


def measure_run(repository, run_id, allow_incomplete):
    metadata = gh_api(f"repos/{repository}/actions/runs/{run_id}")
    if metadata.get("status") != "completed" and not allow_incomplete:
        raise PublicFailure(
            "run_not_completed",
            f"GitHub Actions run {run_id} has status {metadata.get('status')!r}, not 'completed'",
            helper_envelope.EXIT["attention"],
        )
    pages = gh_api(f"repos/{repository}/actions/runs/{run_id}/jobs", paginate=True)
    raw_jobs = []
    for page in pages:
        if isinstance(page, dict):
            raw_jobs.extend(page.get("jobs") or [])
        elif isinstance(page, list):
            raw_jobs.extend(page)
    jobs = [measured_job(job) for job in raw_jobs]
    starts = [parse_time(job["started_at"]) for job in jobs if job["seconds"]]
    finishes = [parse_time(job["completed_at"]) for job in jobs if job["seconds"]]
    wall_seconds = max(0, (max(finishes) - min(starts)).total_seconds()) if starts and finishes else 0
    if float(wall_seconds).is_integer():
        wall_seconds = int(wall_seconds)
    totals = {
        "wall_seconds": wall_seconds,
        "job_seconds": sum(job["seconds"] for job in jobs),
        "billable_minutes": sum(job["billable_minutes"] for job in jobs),
    }
    return {
        "id": metadata.get("id", int(run_id)),
        "status": metadata.get("status", ""),
        "conclusion": metadata.get("conclusion"),
        "head_sha": metadata.get("head_sha", ""),
        "run_attempt": metadata.get("run_attempt"),
        "jobs": jobs,
        "totals": totals,
    }


def main():
    args = parser().parse_args()
    if shutil.which("gh") is None:
        fail("gh_not_found", "gh was not found on PATH", helper_envelope.EXIT["usage"], kind="executable", target_id="gh")
    repository = (args.repo or os.environ.get("REPLAN_GITHUB_REPOSITORY", "")).strip() or repository_from_remote()
    if not re.fullmatch(r"[^/\s]+/[^/\s]+", repository):
        fail(
            "github_repository_not_found",
            "repository must be owner/name via --repo, REPLAN_GITHUB_REPOSITORY, or the GitHub origin remote",
            helper_envelope.EXIT["usage"], kind="repository", target_id=repository,
        )
    if any(not re.fullmatch(r"[0-9]+", run_id) for run_id in args.runs):
        fail("invalid_run_id", "every --run value must be a numeric GitHub Actions run id", helper_envelope.EXIT["usage"])
    try:
        runs = [measure_run(repository, run_id, args.allow_incomplete) for run_id in args.runs]
    except PublicFailure as exc:
        fail(exc.code, str(exc), exc.exit_code)
    totals = {
        name: sum(run["totals"][name] for run in runs)
        for name in ("wall_seconds", "job_seconds", "billable_minutes")
    }
    helper_envelope.emit({
        "ok": True,
        "command": COMMAND,
        "mode": "read_only",
        "repository": repository,
        "run_ids": [str(run_id) for run_id in args.runs],
        "runs": runs,
        "totals": totals,
    }, 0)


if __name__ == "__main__":
    main()
