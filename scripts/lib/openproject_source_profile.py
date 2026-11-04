"""Find the OpenProject source profile selected by helper settings."""

import os

import yaml

import helper_envelope


def load_profiles(project_path):
    path = os.path.join(project_path, ".replan", "project.yaml")
    with open(path, encoding="utf-8") as handle:
        document = yaml.safe_load(handle) or {}
    project = document.get("project") or {}
    return [
        profile
        for profile in project.get("source_profiles") or []
        if isinstance(profile, dict) and profile.get("source") == "openproject"
    ]


def _repair_message(project_id):
    return (
        "Add these YAML lines under project.source_profiles in "
        ".replan/project.yaml:\n"
        f"    - id: openproject.{project_id}\n"
        "      source: openproject\n"
        "      base_url: https://op.example.internal\n"
        f"      project_id: {project_id}\n"
        "      token_env: OPENPROJECT_TOKEN\n"
        "For a new plan, run replan init --openproject-project-id "
        f"{project_id} with the other required init flags."
    )


def check(project_path, target_project_id):
    """Return (matching profile, diagnostic) for a selected project id."""
    profiles = load_profiles(project_path)
    for profile in profiles:
        if str(profile.get("project_id") or "") == target_project_id:
            return profile, None
    if not profiles:
        message = (
            f"the RePlan project has no OpenProject source profile for "
            f"REPLAN_OPENPROJECT_PROJECT_ID {target_project_id!r}. "
            + _repair_message(target_project_id)
        )
        code = "openproject_source_profile_missing"
    else:
        profile_ids = sorted({str(profile.get("project_id") or "<empty>") for profile in profiles})
        message = (
            f"the RePlan project OpenProject source profile project_id "
            f"{', '.join(repr(value) for value in profile_ids)} does not match "
            f"REPLAN_OPENPROJECT_PROJECT_ID {target_project_id!r}. "
            + _repair_message(target_project_id)
        )
        code = "openproject_source_profile_mismatch"
    return None, helper_envelope.diagnostic(
        code,
        message,
        "warning",
        "openproject_project",
        target_project_id,
    )
