"""Moved from scripts/apply-openproject-relations.sh (GAP-018). Invoked as `python3 -m apply_openproject_relations`."""
import json
import os
import sys

import yaml
from openproject_http import OpenProjectClient, Settings
import helper_envelope
import openproject_bindings
import openproject_rescheduling
import openproject_working_days

root_dir, argv = helper_envelope.internal_prefix(
    "apply-openproject-relations", "--root-dir"
)
helper_envelope.internal_parser("apply-openproject-relations").parse_args(argv)
settings = Settings.from_env(require_token=True, root_dir=root_dir)
project_path, base_url = settings.project_path, settings.base_url
token_env, host_header = settings.token_env, settings.host_header
forwarded_proto, openproject_project_id = settings.forwarded_proto, settings.project_id
apply, bindings_path = settings.apply, settings.bindings_path
keep_automatic_scheduling = os.environ.get(
    "REPLAN_OPENPROJECT_KEEP_AUTOMATIC_SCHEDULING", ""
) == "1"
mode = "apply" if apply == "1" else "dry_run"
helper_envelope.guard("apply-openproject-relations", mode)
partial_apply = helper_envelope.PartialApply()
partial_binding_context = {"document": None, "dependencies": []}
base_url = base_url.rstrip("/")
client = OpenProjectClient(
    base_url, settings, host_header, forwarded_proto,
    default_expected=(200, 201, 204, 404),
)

def safe_main(fn):
    try:
        return fn()
    except Exception as exc:
        private_binding_layer = persist_partial_bindings()
        entry, code, extra = partial_apply.failure(exc, "openproject_relation_apply_failed")
        out = {
            "ok": False,
            "command": "apply-openproject-relations",
            "mode": mode,
            "openproject_request": settings.request_metadata(),
            "errors": [entry],
            "private_binding_layer": private_binding_layer,
            **extra,
        }
        if partial_apply.applied_so_far:
            out["operator_next_steps"] = [
                "Run this helper again as a dry run; it matches relations by their endpoint work packages, so confirmed relations are not recreated."
            ]
            if entry.get("outcome_unknown"):
                out["operator_next_steps"].insert(0, "Check the failed relation endpoints in OpenProject first; the relation may have been applied.")
        elif entry.get("outcome_unknown"):
            out["operator_next_steps"] = [
                "Check whether the relation exists before running apply again."
            ]
        helper_envelope.emit(out, code)

def load_yaml(path):
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

def relation_tuple_for_dependency(dep, task_wp_by_id):
    dep_type = dep.get("type", "")
    predecessor_id = (dep.get("predecessor_ref") or {}).get("id", "")
    successor_id = (dep.get("successor_ref") or {}).get("id", "")
    predecessor_wp = task_wp_by_id.get(predecessor_id, "")
    successor_wp = task_wp_by_id.get(successor_id, "")
    if not predecessor_wp or not successor_wp:
        return None, {
            "dependency_id": dep.get("id", ""),
            "reason": "task_binding_missing",
            "predecessor_task_id": predecessor_id,
            "successor_task_id": successor_id,
            "predecessor_work_package_id": predecessor_wp,
            "successor_work_package_id": successor_wp,
        }
    if dep_type == "finish_to_start":
        return {
            "type": "follows",
            "from_work_package_id": successor_wp,
            "to_work_package_id": predecessor_wp,
            "predecessor_task_id": predecessor_id,
            "successor_task_id": successor_id,
        }, None
    if dep_type == "blocks":
        return {
            "type": "blocks",
            "from_work_package_id": predecessor_wp,
            "to_work_package_id": successor_wp,
            "predecessor_task_id": predecessor_id,
            "successor_task_id": successor_id,
        }, None
    return None, {
        "dependency_id": dep.get("id", ""),
        "reason": "unsupported_dependency_type",
        "dependency_type": dep_type,
    }

def list_relations():
    return client.paginate("/api/v3/relations", 500, {})

def relation_tuple(relation):
    links = relation.get("_links") or {}
    from_id = (links.get("from") or {}).get("href", "").rstrip("/").split("/")[-1]
    to_id = (links.get("to") or {}).get("href", "").rstrip("/").split("/")[-1]
    return (relation.get("type", ""), from_id, to_id)

def create_relation(rel):
    payload = {
        "type": rel["type"],
        "_links": {
            "from": {"href": f"/api/v3/work_packages/{rel['from_work_package_id']}"},
            "to": {"href": f"/api/v3/work_packages/{rel['to_work_package_id']}"},
        },
    }
    path = f"/api/v3/work_packages/{rel['from_work_package_id']}/relations"
    _, data = client.request_json("POST", path, payload, expected=(201,))
    return data

def source_ref_for_relation(relation_id):
    return {
        "source": "openproject",
        "object_type": "relation",
        "object_id": str(relation_id),
        "url": f"{base_url}/relations/{relation_id}",
    }


def persist_partial_bindings():
    document = partial_binding_context["document"]
    new_dependencies = partial_binding_context["dependencies"]
    summary = {
        "path": bindings_path,
        "written": False,
        "dependency_bindings": len(new_dependencies),
    }
    if mode != "apply" or document is None or not new_dependencies:
        return summary
    existing = openproject_bindings.root(document).get("dependencies") or []
    merged = {
        item.get("replan_dependency_id", ""): item
        for item in existing
        if isinstance(item, dict) and item.get("replan_dependency_id")
    }
    for item in new_dependencies:
        merged[item["replan_dependency_id"]] = item
    try:
        openproject_bindings.replace_dependency_bindings(
            bindings_path, list(merged.values()), document=document
        )
    except Exception as exc:
        summary["write_error"] = str(exc)
        return summary
    summary["written"] = True
    summary["dependency_bindings"] = len(merged)
    return summary

def main():
    dependencies_path = os.path.join(project_path, "canonical", "dependencies.yaml")
    dependencies = load_yaml(dependencies_path).get("dependencies") or []
    bindings_doc = openproject_bindings.load(bindings_path, required=True)
    partial_binding_context["document"] = bindings_doc
    task_wp_by_id, task_id_by_wp = openproject_bindings.task_maps(
        bindings_doc, zero_is_missing=True
    )

    existing_relations = list_relations()
    existing_by_tuple = {}
    for relation in existing_relations:
        existing_by_tuple[relation_tuple(relation)] = relation

    existing_private_deps = openproject_bindings.dependency_bindings_by_id(bindings_doc)
    actions = []
    errors = []
    new_private_deps = []
    partial_binding_context["dependencies"] = new_private_deps

    managed_follows = []
    pending_follows = []
    for dep in dependencies:
        rel, err = relation_tuple_for_dependency(dep, task_wp_by_id)
        if err or rel["type"] != "follows":
            continue
        managed_follows.append({**rel, "replan_dependency_id": dep.get("id", "")})
        key = (rel["type"], rel["from_work_package_id"], rel["to_work_package_id"])
        if key not in existing_by_tuple:
            pending_follows.append({**rel, "replan_dependency_id": dep.get("id", "")})

    rescheduling_targets = set()
    rescheduling_before = {}
    successors = {}
    pending_successors = {
        rel["from_work_package_id"] for rel in pending_follows
    }
    managed_successors = {
        rel["from_work_package_id"] for rel in managed_follows
    }
    if managed_follows:
        graph_relations = [
            {"type": kind, "from_work_package_id": from_id, "to_work_package_id": to_id}
            for kind, from_id, to_id in existing_by_tuple
        ] + pending_follows
        successors = openproject_rescheduling.follows_successors(graph_relations)
        rescheduling_targets = (
            openproject_rescheduling.reachable_work_packages(
                managed_successors, successors
            )
            & set(task_id_by_wp)
        )
        rescheduling_before, rescheduling_initial_unread = (
            openproject_rescheduling.project_work_package_dates(
                client, openproject_project_id, rescheduling_targets,
                tolerate_failures=(mode == "dry_run"),
            )
        )
    else:
        rescheduling_initial_unread = []

    planned_restores = set()
    if not keep_automatic_scheduling:
        planned_restores.update(pending_successors & rescheduling_targets)
        planned_restores.update(
            work_package_id
            for work_package_id, snapshot in rescheduling_before.items()
            if snapshot.get("scheduleManually") is False
        )

    for dep in dependencies:
        dep_id = dep.get("id", "")
        rel, err = relation_tuple_for_dependency(dep, task_wp_by_id)
        if err:
            errors.append(err)
            continue
        key = (rel["type"], rel["from_work_package_id"], rel["to_work_package_id"])
        existing = existing_by_tuple.get(key)
        if existing:
            relation_id = str(existing.get("id", ""))
            action = "match_existing_relation"
        elif mode == "apply":
            step = {
                "action": "create_relation",
                "replan_dependency_id": dep_id,
                "relation_type": rel["type"],
                "from_work_package_id": rel["from_work_package_id"],
                "to_work_package_id": rel["to_work_package_id"],
            }
            partial_apply.attempting(step)
            partial_apply.writing()
            created = create_relation(rel)
            relation_id = str(created.get("id", ""))
            existing_by_tuple[key] = created
            action = "created_relation"
        else:
            relation_id = ""
            action = "create_relation"
        action_item = {
            "action": action,
            "replan_dependency_id": dep_id,
            "relation_type": rel["type"],
            "from_work_package_id": rel["from_work_package_id"],
            "to_work_package_id": rel["to_work_package_id"],
            "predecessor_task_id": rel["predecessor_task_id"],
            "successor_task_id": rel["successor_task_id"],
            "openproject_relation_id": relation_id,
        }
        actions.append(action_item)
        if relation_id:
            previous = existing_private_deps.get(dep_id, {})
            new_private_deps.append({
                "replan_dependency_id": dep_id,
                "source_ref": source_ref_for_relation(relation_id),
                "relation_type": rel["type"],
                "from_work_package_id": rel["from_work_package_id"],
                "to_work_package_id": rel["to_work_package_id"],
                "predecessor_task_id": rel["predecessor_task_id"],
                "successor_task_id": rel["successor_task_id"],
                "notes": previous.get("notes", "Private live OpenProject relation binding."),
            })
        if action == "created_relation":
            partial_apply.applied(action_item)

    bindings_written = False
    if mode == "apply" and not errors:
        binding_step = {
            "action": "write_private_bindings",
            "path": bindings_path,
            "dependency_bindings": len(new_private_deps),
        }
        partial_apply.attempting(binding_step)
        partial_apply.writing()
        openproject_bindings.replace_dependency_bindings(
            bindings_path, new_private_deps, document=bindings_doc
        )
        bindings_written = True
        partial_apply.applied({**binding_step, "written": True})

    restored = []
    rescheduled = []
    rescheduling_unread = list(rescheduling_initial_unread)
    if rescheduling_before:
        rescheduling_after, post_create_unread = openproject_rescheduling.project_work_package_dates(
            client, openproject_project_id, rescheduling_targets, tolerate_failures=True
        )
        rescheduling_unread.extend(post_create_unread)
        restore_targets = set()
        if not keep_automatic_scheduling:
            for work_package_id in set(rescheduling_before) & set(rescheduling_after):
                before = rescheduling_before[work_package_id]
                after = rescheduling_after[work_package_id]
                if (
                    before.get("scheduleManually") is False
                    or before.get("scheduleManually") != after.get("scheduleManually")
                    or before.get("startDate", "") != after.get("startDate", "")
                    or before.get("dueDate", "") != after.get("dueDate", "")
                ):
                    restore_targets.add(work_package_id)

        working_day_warnings = []
        if mode == "apply" and restore_targets:
            working_by_day, working_day_warnings = openproject_working_days.load(client)
            for work_package_id in openproject_rescheduling.roots_first(
                restore_targets, successors
            ):
                before = rescheduling_before[work_package_id]
                current_by_id, _ = openproject_rescheduling.work_package_dates(
                    client, [work_package_id]
                )
                current = current_by_id[work_package_id]
                step = {
                    "action": "restore_manual_schedule",
                    "work_package_id": work_package_id,
                    "task_id": task_id_by_wp.get(work_package_id, ""),
                    "before": {
                        "scheduleManually": current.get("scheduleManually"),
                        "startDate": current.get("startDate", ""),
                        "dueDate": current.get("dueDate", ""),
                    },
                    "after": {
                        "scheduleManually": True,
                        "startDate": before.get("startDate", ""),
                        "dueDate": before.get("dueDate", ""),
                    },
                }
                payload = openproject_working_days.manual_schedule_payload(
                    current.get("lockVersion"),
                    before.get("startDate", ""),
                    before.get("dueDate", ""),
                    step,
                    working_by_day,
                    include_dates=True,
                )
                partial_apply.attempting(step)
                partial_apply.writing()
                _, updated = client.request_json(
                    "PATCH", f"/api/v3/work_packages/{work_package_id}",
                    payload, expected=(200,),
                )
                applied_restore = {
                    **step,
                    "after": {
                        "scheduleManually": updated.get("scheduleManually"),
                        "startDate": updated.get("startDate", ""),
                        "dueDate": updated.get("dueDate", ""),
                    },
                }
                restored.append(applied_restore)
                partial_apply.applied(applied_restore)

            verified_after, verify_unread = openproject_rescheduling.project_work_package_dates(
                client, openproject_project_id, rescheduling_targets,
                tolerate_failures=True,
            )
            rescheduling_after = verified_after
            rescheduling_unread.extend(verify_unread)

        causes = {}
        for action in actions:
            if action["action"] != "created_relation" or action["relation_type"] != "follows":
                continue
            for affected in openproject_rescheduling.reachable_work_packages(
                [action["from_work_package_id"]], successors
            ):
                causes.setdefault(affected, []).append({
                    "caused_by_relation": {
                        "replan_dependency_id": action["replan_dependency_id"],
                        "openproject_relation_id": action["openproject_relation_id"],
                    }
                })
        rescheduled = openproject_rescheduling.moved_dates(
            rescheduling_before, rescheduling_after, task_id_by_wp, causes=causes
        )
        if mode == "apply" and not keep_automatic_scheduling:
            for work_package_id in sorted(restore_targets & set(rescheduling_after)):
                after = rescheduling_after[work_package_id]
                if after.get("scheduleManually") is not True:
                    rescheduled.append({
                        "task_id": task_id_by_wp.get(work_package_id, ""),
                        "work_package_id": work_package_id,
                        "field": "scheduleManually",
                        "before": True,
                        "after": after.get("scheduleManually"),
                    })
    else:
        working_day_warnings = []

    if mode == "dry_run" and planned_restores:
        for work_package_id in openproject_rescheduling.roots_first(
            planned_restores, successors
        ):
            before = rescheduling_before.get(work_package_id, {})
            restored.append({
                "action": "restore_manual_schedule",
                "work_package_id": work_package_id,
                "task_id": task_id_by_wp.get(work_package_id, ""),
                "before": {
                    "scheduleManually": before.get("scheduleManually"),
                    "startDate": before.get("startDate", ""),
                    "dueDate": before.get("dueDate", ""),
                },
                "after": {
                    "scheduleManually": True,
                    "startDate": before.get("startDate", ""),
                    "dueDate": before.get("dueDate", ""),
                },
            })

    counts = {
        "planned_dependencies": len(dependencies),
        "actions": len(actions),
        "create_relation": sum(1 for action in actions if action["action"] == "create_relation"),
        "created_relation": sum(1 for action in actions if action["action"] == "created_relation"),
        "matched_existing_relation": sum(1 for action in actions if action["action"] == "match_existing_relation"),
        "errors": len(errors),
    }
    out = {
        "ok": len(errors) == 0,
        "command": "apply-openproject-relations",
        "mode": mode,
        "project_path": project_path,
        "target_openproject_project": {
            "identifier": openproject_project_id,
        },
        "openproject_request": settings.request_metadata(),
        "private_binding_layer": {
            "path": bindings_path,
            "written": bindings_written,
            "dependency_bindings": len(new_private_deps),
        },
        "counts": counts,
        "actions": actions,
        "restored_work_packages": restored,
        "errors": errors,
        "operator_next_steps": [
            "If this was dry-run and actions look correct, run with REPLAN_APPLY=1.",
            "After apply, run check-openproject-live-alignment and validate-openproject-live.",
            "Keep generated private relation bindings out of git.",
        ],
    }
    if mode == "dry_run" and pending_follows:
        out["operator_next_steps"].insert(
            1, "Apply will restore affected bound successors to manual scheduling unless REPLAN_OPENPROJECT_KEEP_AUTOMATIC_SCHEDULING=1."
        )
    if working_day_warnings:
        out.setdefault("warnings", []).extend(working_day_warnings)
    if rescheduled:
        out["rescheduled_work_packages"] = rescheduled
        moved_count = len({item["work_package_id"] for item in rescheduled})
        out.setdefault("warnings", []).append({
            **helper_envelope.diagnostic(
                "openproject_rescheduled",
                f"OpenProject rescheduled {moved_count} work package(s) after relation creation.",
                "warning",
            ),
            "count": moved_count,
        })
        out["operator_next_steps"] = [
            "Run scripts/check-openproject-drift.sh to review the date changes.",
            "If RePlan dates should win, run scripts/push-openproject-date-drift.sh as a dry run, then apply it with REPLAN_APPLY=1.",
            "Otherwise accept OpenProject's scheduler dates into RePlan through the reviewed soft-drift flow.",
        ]
    if rescheduling_unread:
        out["ok"] = False
        out.setdefault("warnings", []).append({
            **helper_envelope.diagnostic(
                "rescheduling_unknown",
                "Relations were created, but some affected work packages could not be read afterward.",
                "warning",
            ),
            "work_package_ids": sorted(set(rescheduling_unread)),
        })
        out["operator_next_steps"] = [
            "Inspect the unread work packages in OpenProject, then run scripts/check-openproject-drift.sh.",
            "If RePlan dates should win, run scripts/push-openproject-date-drift.sh as a dry run, then apply it with REPLAN_APPLY=1.",
        ]
    applied = sum(1 for action in actions if action["action"] == "created_relation")
    exit_code = helper_envelope.EXIT["attention"] if rescheduling_unread else helper_envelope.apply_exit_code(out, applied)
    helper_envelope.emit(out, exit_code, mode)

safe_main(main)
