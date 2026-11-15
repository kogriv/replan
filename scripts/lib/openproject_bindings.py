"""Read and write the ignored OpenProject private binding layer."""

import datetime as dt
import os
import tempfile

import yaml


ROOT_KEY = "openproject_private_bindings"
LIST_KEYS = ("tasks", "dependencies", "resources")


def dependency_expected_in_openproject(dependency):
    """Return whether a canonical dependency should have a live relation.

    Resolved dependencies remain expected as relation history.  Only a
    cancelled dependency is removed from the OpenProject relation skeleton.
    """
    return str(dependency.get("status") or "") != "cancelled"


def dependency_endpoints(dependency):
    return (
        str((dependency.get("predecessor_ref") or {}).get("id") or ""),
        str((dependency.get("successor_ref") or {}).get("id") or ""),
    )


def cancelled_dependency_warning(
    dependency, relation_id, *, live=False, predecessor_work_package_id="",
    successor_work_package_id="", from_work_package_id="",
    to_work_package_id="",
):
    """Build the common advisory for cancelled relation cleanup."""
    dependency_id = str(dependency.get("id") or "")
    predecessor, successor = dependency_endpoints(dependency)
    relation_id = str(relation_id or "")
    delete_command = (
        "scripts/openproject-work-package-toolkit.sh relation delete "
        f"--relation {relation_id} --remove-private-binding"
    )
    if live:
        code = "cancelled_dependency_still_live"
        message = (
            f"Cancelled dependency {dependency_id} still has live OpenProject "
            f"relation {relation_id}; review and delete it explicitly."
        )
        hint = "Run the delete command after reviewing the relation endpoints."
    else:
        code = "cancelled_dependency_binding_left"
        message = (
            f"Cancelled dependency {dependency_id} still has private relation "
            f"binding {relation_id}, but that relation is no longer live."
        )
        hint = (
            "Run the delete command even though the relation is already gone; "
            "the toolkit accepts OpenProject's 404 and removes the private binding."
        )
    return {
        "code": code,
        "message": message,
        "severity": "warning",
        "target": {"kind": "dependency", "id": dependency_id},
        "dependency_id": dependency_id,
        "relation_id": relation_id,
        "dependency_type": str(dependency.get("type") or ""),
        "predecessor_task_id": predecessor,
        "successor_task_id": successor,
        "predecessor_work_package_id": str(predecessor_work_package_id or ""),
        "successor_work_package_id": str(successor_work_package_id or ""),
        "from_work_package_id": str(from_work_package_id or ""),
        "to_work_package_id": str(to_work_package_id or ""),
        "delete_command": delete_command,
        "operator_hint": hint,
    }


class BindingsValidationError(ValueError):
    def __init__(self, reason, key=""):
        super().__init__(reason)
        self.reason = reason
        self.key = key


def empty_document():
    return {ROOT_KEY: {key: [] for key in LIST_KEYS}}


def load(path, *, required=False, with_state=False, validate=False):
    """Load a binding document while retaining fields this module does not own."""
    if not os.path.exists(path):
        if required:
            raise FileNotFoundError(f"private binding file not found: {path}")
        document = empty_document()
        return (document, {"exists": False, "root_loaded": False}) if with_state else document
    with open(path, encoding="utf-8") as handle:
        document = yaml.safe_load(handle) or {}
    if validate:
        if not isinstance(document, dict) or not isinstance(document.get(ROOT_KEY), dict):
            raise BindingsValidationError("missing_root")
        for key in LIST_KEYS:
            if key in document[ROOT_KEY] and not isinstance(document[ROOT_KEY][key], list):
                raise BindingsValidationError("non_list", key)
    if not isinstance(document, dict):
        document = {}
    loaded_root = document.get(ROOT_KEY)
    root_loaded = bool(loaded_root) if isinstance(loaded_root, dict) else False
    if not isinstance(loaded_root, dict):
        loaded_root = {}
        document[ROOT_KEY] = loaded_root
    root = loaded_root
    for key in LIST_KEYS:
        if root.get(key) is None:
            root[key] = []
    return (document, {"exists": True, "root_loaded": root_loaded}) if with_state else document


def root(document):
    return document[ROOT_KEY]


def project(document):
    value = root(document).get("project")
    return value if isinstance(value, dict) else {}


def _accepted_ref(source_ref, object_type, strict):
    if not strict:
        return True
    return source_ref.get("source") == "openproject" and source_ref.get("object_type") == object_type


def _object_id(source_ref, zero_is_missing):
    value = source_ref.get("object_id")
    if zero_is_missing:
        return str(value or "")
    return "" if value is None else str(value)


def task_maps(
    document, *, strict=True, zero_is_missing=False,
    duplicate_task_errors=None, duplicate_work_package_errors=None,
):
    task_to_work_package = {}
    work_package_to_task = {}
    for binding in root(document).get("tasks") or []:
        task_id = binding.get("replan_task_id", "")
        source_ref = binding.get("source_ref") or {}
        if not _accepted_ref(source_ref, "work_package", strict):
            continue
        work_package_id = _object_id(source_ref, zero_is_missing)
        if not task_id or not work_package_id:
            continue
        if duplicate_task_errors is not None and task_id in task_to_work_package and task_to_work_package[task_id] != work_package_id:
            duplicate_task_errors.append({"kind": "duplicate_task_binding", "task_id": task_id})
        if duplicate_work_package_errors is not None and work_package_id in work_package_to_task and work_package_to_task[work_package_id] != task_id:
            duplicate_work_package_errors.append({"kind": "duplicate_work_package_binding", "work_package_id": work_package_id})
        task_to_work_package[task_id] = work_package_id
        work_package_to_task[work_package_id] = task_id
    return task_to_work_package, work_package_to_task


def dependency_maps(document, *, strict=True, zero_is_missing=False):
    dependency_to_relation = {}
    relation_to_dependency = {}
    for binding in root(document).get("dependencies") or []:
        dependency_id = binding.get("replan_dependency_id", "")
        source_ref = binding.get("source_ref") or {}
        if not _accepted_ref(source_ref, "relation", strict):
            continue
        relation_id = _object_id(source_ref, zero_is_missing)
        if dependency_id and relation_id:
            dependency_to_relation[dependency_id] = relation_id
            relation_to_dependency[relation_id] = dependency_id
    return dependency_to_relation, relation_to_dependency


def dependency_bindings_by_id(document):
    return {
        binding.get("replan_dependency_id", ""): binding
        for binding in root(document).get("dependencies") or []
        if binding.get("replan_dependency_id", "")
    }


def task_source_refs(document, *, strict=True):
    """Return task refs; permissive mode preserves the overlay's old defaults."""
    refs = {}
    for binding in root(document).get("tasks") or []:
        task_id = binding.get("replan_task_id")
        source_ref = binding.get("source_ref") or {}
        if not _accepted_ref(source_ref, "work_package", strict):
            continue
        if task_id and source_ref.get("object_id"):
            refs[task_id] = {
                "source": source_ref.get("source", "openproject"),
                "object_type": source_ref.get("object_type", "work_package"),
                "object_id": str(source_ref["object_id"]),
                "url": source_ref.get("url", ""),
            }
    return refs


def refresh_generated_at(document):
    root(document)["generated_at"] = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def write(path, document):
    """Atomically replace a binding file with a complete YAML dump."""
    bindings_dir = os.path.dirname(path) or "."
    os.makedirs(bindings_dir, exist_ok=True)
    temp_path = None
    try:
        with tempfile.NamedTemporaryFile("w", dir=bindings_dir, delete=False, encoding="utf-8") as handle:
            temp_path = handle.name
            yaml.safe_dump(document, handle, sort_keys=False, allow_unicode=True)
        os.replace(temp_path, path)
        temp_path = None
    finally:
        if temp_path is not None:
            try:
                os.unlink(temp_path)
            except FileNotFoundError:
                pass


def append_task_binding(path, task_id, source_ref, *, document=None):
    document = document if document is not None else load(path)
    root(document)["tasks"].append({"replan_task_id": task_id, "source_ref": source_ref})
    refresh_generated_at(document)
    write(path, document)
    return document


def replace_dependency_bindings(path, dependencies, *, document=None):
    document = document if document is not None else load(path)
    root(document)["dependencies"] = dependencies
    refresh_generated_at(document)
    write(path, document)
    return document
