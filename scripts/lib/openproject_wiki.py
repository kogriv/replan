"""Wiki pages and their attachments of an OpenProject project, over API v3.

Invoked by scripts/openproject-wiki.sh as `python3 -m openproject_wiki`. Pages
go through the routes of the OpenProject wiki API plugin; attachments through
OpenProject's own attachment routes. Everything happens as the token's own
user, under that user's wiki permissions.

A command that would change the wiki is a dry run unless REPLAN_APPLY=1. A
page is never overwritten blindly: an update names the lock version it was
read at, or reads it just before writing, and a page changed in between is
refused, not merged. Page text is user content: it is written to the file the
caller names or returned by `page show`, and never put into an error.
"""

import argparse
import difflib
import hashlib
import json
import mimetypes
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

import helper_envelope
from openproject_http import (
    OpenProjectClient,
    Settings,
    openproject_request_json,
    summarize_error_body,
)

argv = sys.argv[1:]
settings = None
base_url = token_env = project_id = host_header = forwarded_proto = token = ""
mode = "dry_run"
client = None
partial_apply = None


def initialize_environment():
    global settings, base_url, token_env, project_id, host_header
    global forwarded_proto, token, mode, client
    settings = Settings.from_env()
    base_url, token_env = settings.base_url.rstrip("/"), settings.token_env
    project_id, host_header = settings.project_id, settings.host_header
    forwarded_proto, token = settings.forwarded_proto, settings.token
    mode = "apply" if settings.apply == "1" else "dry_run"
    helper_envelope.guard("openproject wiki", mode)
    client = OpenProjectClient(base_url, settings, host_header, forwarded_proto)


def diagnostic(code, message, severity="error", kind="", target_id=""):
    out = {"code": code, "message": message, "severity": severity}
    if kind or target_id:
        out["target"] = {"kind": kind, "id": str(target_id)}
    return out


def envelope(command, command_mode, *, ok=True, items=None, actions=None, counts=None, warnings=None, errors=None, extra=None):
    warnings = warnings or []
    errors = errors or []
    out = {
        "ok": bool(ok and not errors),
        "command": command,
        "mode": command_mode,
        "target_openproject_project": {"identifier": project_id},
        "openproject_request": settings.request_metadata(),
        "counts": counts or {},
        "items": items or [],
        "actions": actions or [],
        "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
        "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
        "warnings": warnings,
        "errors": errors,
    }
    if extra:
        out.update(extra)
    return out


def print_json(data, exit_code=None):
    helper_envelope.emit(data, exit_code, mode, sort_keys=True)


def require_settings(command):
    missing = []
    if not base_url:
        missing.append(diagnostic("missing_base_url", "REPLAN_OPENPROJECT_BASE_URL is not set", "error", "env", "REPLAN_OPENPROJECT_BASE_URL"))
    if not token:
        missing.append(diagnostic("token_env_not_set", f"{token_env} is not set", "error", "env", token_env))
    if not project_id:
        missing.append(diagnostic("missing_project_id", "REPLAN_OPENPROJECT_PROJECT_ID is not set", "error", "env", "REPLAN_OPENPROJECT_PROJECT_ID"))
    if missing:
        print_json(envelope(command, mode, ok=False, errors=missing), helper_envelope.EXIT["usage"])


def http_error(exc, method, path):
    """A RuntimeError like openproject_request_json raises, body reduced."""
    response = summarize_error_body(exc.read().decode(errors="replace"))
    return RuntimeError(json.dumps({
        "code": "openproject_http_error", "method": method, "path": path, "status": exc.code,
        "message": f"{method} {path} returned HTTP {exc.code}", "response": response,
    }, ensure_ascii=False))


def error_status(exc):
    try:
        return json.loads(str(exc)).get("status")
    except Exception:
        return None


def project_path():
    return "/api/v3/projects/" + urllib.parse.quote(project_id)


# Pages

def page_summary(page):
    links = page.get("_links") or {}
    parent = links.get("parent") or {}
    return {
        "id": page.get("id"),
        "title": page.get("title", ""),
        "slug": page.get("slug", ""),
        "lock_version": page.get("lockVersion"),
        "protected": bool(page.get("protected")),
        "parent_id": id_from_href(parent.get("href")),
        "parent_title": parent.get("title", "") if parent.get("href") else "",
        "updated_at": page.get("updatedAt", ""),
    }


def id_from_href(href):
    if not href:
        return None
    tail = href.rstrip("/").rsplit("/", 1)[-1]
    return int(tail) if tail.isdigit() else None


def all_pages():
    return client.paginate(f"{project_path()}/wiki_pages", 100, {})


def find_page_id(args, command):
    """The page an --id, --slug or --title names; a title must be unambiguous."""
    if args.id:
        return int(args.id)
    matches = [
        page for page in all_pages()
        if (args.slug and page.get("slug") == args.slug) or (args.title and page.get("title") == args.title)
    ]
    wanted = args.slug or args.title
    if not matches:
        print_json(envelope(command, mode_of(command), errors=[diagnostic("wiki_page_not_found", f"no wiki page named {wanted!r} in project {project_id}", "error", "wiki_page", wanted)]), 2)
    if len(matches) > 1:
        ids = ", ".join(str(page.get("id")) for page in matches)
        print_json(envelope(command, mode_of(command), errors=[diagnostic("wiki_page_ambiguous", f"{len(matches)} wiki pages are named {wanted!r}: ids {ids}; use --id", "error", "wiki_page", wanted)]), 2)
    return int(matches[0]["id"])


def mode_of(command):
    return "read_only" if command.endswith((" list", " show", " download")) else mode


def read_page(page_id, command):
    try:
        _, page = client.request_json("GET", f"/api/v3/wiki_pages/{page_id}", expected=(200,))
    except RuntimeError as exc:
        if error_status(exc) == 404:
            print_json(envelope(command, mode_of(command), errors=[diagnostic("wiki_page_not_found", f"wiki page {page_id} was not found, or this user may not see it", "error", "wiki_page", page_id)]), 2)
        raise
    return page


def page_text(page):
    return ((page.get("text") or {}).get("raw")) or ""


def read_text_file(path, command):
    try:
        with open(path, encoding="utf-8") as handle:
            return handle.read()
    except OSError as exc:
        print_json(envelope(command, mode, errors=[diagnostic("text_file_unreadable", f"cannot read {path}: {exc.strerror}", "error", "file", path)]), 2)


def text_facts(text):
    return {"bytes": len(text.encode()), "lines": len(text.splitlines()), "sha256": hashlib.sha256(text.encode()).hexdigest()}


def command_page_list(_args):
    command = "openproject wiki page list"
    require_settings(command)
    items = [page_summary(page) for page in all_pages()]
    print_json(envelope(command, "read_only", items=items, counts={"pages": len(items)}))


def command_page_show(args):
    command = "openproject wiki page show"
    require_settings(command)
    page = read_page(find_page_id(args, command), command)
    text = page_text(page)
    item = page_summary(page)
    item["text"] = text_facts(text)
    item["attachments"] = [attachment_summary(a) for a in embedded_attachments(page)]
    extra = {}
    if args.output:
        with open(args.output, "w", encoding="utf-8") as handle:
            handle.write(text)
        item["text"]["written_to"] = args.output
    else:
        extra["text"] = text
    print_json(envelope(command, "read_only", items=[item], counts={"pages": 1, "attachments": len(item["attachments"])}, extra=extra))


def command_page_create(args):
    command = "openproject wiki page create"
    require_settings(command)
    text = read_text_file(args.text_file, command) if args.text_file else ""
    same_title = [page for page in all_pages() if page.get("title") == args.title]
    action = {"action": "create_wiki_page", "title": args.title, "text": text_facts(text), "parent_id": args.parent_id or None, "dry_run": mode != "apply"}
    if same_title:
        action["blocked"] = True
        print_json(envelope(command, mode, actions=[action], counts={"planned_actions": 0, "blocked_actions": 1}, errors=[
            diagnostic("wiki_page_title_taken", f"a wiki page titled {args.title!r} already exists (id {same_title[0].get('id')}); update it instead", "error", "wiki_page", same_title[0].get("id")),
        ]), 2)
    if mode != "apply":
        print_json(envelope(command, "dry_run", actions=[action], counts={"planned_actions": 1, "blocked_actions": 0}, extra={"dry_run": True, "suggested_next_actions": ["Rerun with REPLAN_APPLY=1 to create the page."]}))
    payload = {"title": args.title, "text": {"raw": text}}
    if args.parent_id:
        payload["_links"] = {"parent": {"href": f"/api/v3/wiki_pages/{int(args.parent_id)}"}}
    _, created = client.request_json("POST", f"{project_path()}/wiki_pages", payload, expected=(201,))
    action.update({"dry_run": False, "applied": True, "page_id": created.get("id"), "lock_version": created.get("lockVersion")})
    print_json(envelope(command, "apply", actions=[action], items=[page_summary(created)], counts={"applied_actions": 1}))


def command_page_update(args):
    command = "openproject wiki page update"
    require_settings(command)
    page_id = find_page_id(args, command)
    current = read_page(page_id, command)
    old = page_text(current)
    new = read_text_file(args.text_file, command) if args.text_file else old
    old_lines, new_lines = old.splitlines(), new.splitlines()
    diff = list(difflib.unified_diff(old_lines, new_lines, "current", "new", lineterm=""))
    added = sum(1 for line in diff if line.startswith("+") and not line.startswith("+++"))
    removed = sum(1 for line in diff if line.startswith("-") and not line.startswith("---"))
    current_parent = id_from_href(((current.get("_links") or {}).get("parent") or {}).get("href"))
    parent_change = args.parent_id != "" and (int(args.parent_id) if args.parent_id not in ("", "none") else None) != current_parent
    action = {
        "action": "update_wiki_page", "page_id": page_id, "title": current.get("title", ""),
        "lock_version": current.get("lockVersion"), "text_changed": old != new,
        "lines_added": added, "lines_removed": removed, "from": text_facts(old), "to": text_facts(new),
        "parent_changed": parent_change, "dry_run": mode != "apply",
    }
    if args.diff_output:
        with open(args.diff_output, "w", encoding="utf-8") as handle:
            handle.write("\n".join(diff) + ("\n" if diff else ""))
        action["diff_written_to"] = args.diff_output
    if args.expect_lock_version != "" and int(args.expect_lock_version) != current.get("lockVersion"):
        action["blocked"] = True
        print_json(envelope(command, mode, actions=[action], counts={"planned_actions": 0, "blocked_actions": 1}, errors=[
            diagnostic("wiki_page_changed", f"wiki page {page_id} is at lock version {current.get('lockVersion')}, not {args.expect_lock_version}: someone changed it since it was read; read it again and redo the edit", "error", "wiki_page", page_id),
        ]), 2)
    if old == new and not parent_change:
        print_json(envelope(command, mode, actions=[], counts={"planned_actions": 0, "unchanged": 1}, extra={"dry_run": mode != "apply"}))
    if mode != "apply":
        print_json(envelope(command, "dry_run", actions=[action], counts={"planned_actions": 1, "blocked_actions": 0}, extra={"dry_run": True, "suggested_next_actions": ["Review the change (see --diff-output) and rerun with REPLAN_APPLY=1."]}))
    payload = {"lockVersion": current.get("lockVersion")}
    if old != new:
        payload["text"] = {"raw": new}
    if parent_change:
        payload["_links"] = {"parent": {"href": None if args.parent_id == "none" else f"/api/v3/wiki_pages/{int(args.parent_id)}"}}
    try:
        _, updated = client.request_json("PATCH", f"/api/v3/wiki_pages/{page_id}", payload, expected=(200,))
    except RuntimeError as exc:
        error = json.loads(str(exc)) if error_status(exc) == 409 else {}
        if error.get("status") == 409 and not error.get("outcome_unknown"):
            action["blocked"] = True
            print_json(envelope(command, "apply", actions=[action], counts={"applied_actions": 0, "blocked_actions": 1}, errors=[
                diagnostic("wiki_page_changed", f"wiki page {page_id} was changed while this update was being written; nothing was overwritten; read it again and redo the edit", "error", "wiki_page", page_id),
            ]), 2)
        raise
    action.update({"dry_run": False, "applied": True, "new_lock_version": updated.get("lockVersion")})
    print_json(envelope(command, "apply", actions=[action], items=[page_summary(updated)], counts={"applied_actions": 1}))


def command_page_delete(args):
    command = "openproject wiki page delete"
    require_settings(command)
    page_id = find_page_id(args, command)
    page = read_page(page_id, command)
    children = [child for child in all_pages() if id_from_href(((child.get("_links") or {}).get("parent") or {}).get("href")) == page_id]
    action = {"action": "delete_wiki_page", "page_id": page_id, "title": page.get("title", ""), "children": len(children),
              "attachments": len(embedded_attachments(page)), "dry_run": mode != "apply"}
    if children:
        action["blocked"] = True
        print_json(envelope(command, mode, actions=[action], counts={"planned_actions": 0, "blocked_actions": 1}, errors=[
            diagnostic("wiki_page_has_children", f"wiki page {page_id} has {len(children)} child pages; move or delete them first", "error", "wiki_page", page_id),
        ]), 2)
    if mode != "apply":
        print_json(envelope(command, "dry_run", actions=[action], counts={"planned_actions": 1, "blocked_actions": 0}, extra={"dry_run": True, "suggested_next_actions": ["Deleting removes the page, its history and its attachments. Rerun with REPLAN_APPLY=1 to delete."]}))
    client.request_json("DELETE", f"/api/v3/wiki_pages/{page_id}", expected=(204,))
    action.update({"dry_run": False, "applied": True})
    print_json(envelope(command, "apply", actions=[action], counts={"applied_actions": 1}))


# Attachments

def embedded_attachments(page):
    return (((page.get("_embedded") or {}).get("attachments") or {}).get("_embedded") or {}).get("elements") or []


def attachment_summary(attachment):
    return {
        "id": attachment.get("id"),
        "file_name": attachment.get("fileName", ""),
        "file_size": attachment.get("fileSize"),
        "content_type": attachment.get("contentType", ""),
        "md5": (attachment.get("digest") or {}).get("hash", ""),
        "created_at": attachment.get("createdAt", ""),
    }


def page_attachments(page_id):
    return client.paginate(f"/api/v3/wiki_pages/{page_id}/attachments", 100, {})


def command_attachment_list(args):
    command = "openproject wiki attachment list"
    require_settings(command)
    page_id = find_page_id(args, command)
    read_page(page_id, command)
    items = [attachment_summary(a) for a in page_attachments(page_id)]
    print_json(envelope(command, "read_only", items=items, counts={"attachments": len(items)}, extra={"page_id": page_id}))


def command_attachment_download(args):
    command = "openproject wiki attachment download"
    require_settings(command)
    path = f"/api/v3/attachments/{int(args.attachment)}/content"
    req = urllib.request.Request(base_url + path, method="GET", headers=client.headers(content_type=""))
    digest = hashlib.sha256()
    size = 0
    try:
        with urllib.request.urlopen(req, timeout=settings.timeout_seconds) as resp, open(args.output, "wb") as out:
            while True:
                chunk = resp.read(1 << 16)
                if not chunk:
                    break
                out.write(chunk)
                digest.update(chunk)
                size += len(chunk)
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            print_json(envelope(command, "read_only", errors=[diagnostic("attachment_not_found", f"attachment {args.attachment} was not found, or this user may not see it", "error", "attachment", args.attachment)]), 2)
        raise http_error(exc, "GET", path) from None
    except (urllib.error.URLError, TimeoutError, ConnectionError) as exc:
        # A download cut short leaves no half file behind.
        if os.path.exists(args.output):
            os.remove(args.output)
        reason = getattr(exc, "reason", exc)
        raise RuntimeError(json.dumps({
            "code": "openproject_network_error", "method": "GET", "path": path,
            "message": f"the download did not complete: {reason}",
        }, ensure_ascii=False)) from None
    except OSError as exc:
        print_json(envelope(command, "read_only", errors=[diagnostic("output_unwritable", f"cannot write {args.output}: {exc.strerror}", "error", "file", args.output)]), 2)
    item = {"id": int(args.attachment), "written_to": args.output, "bytes": size, "sha256": digest.hexdigest()}
    print_json(envelope(command, "read_only", items=[item], counts={"attachments": 1}))


def multipart(file_name, content, content_type):
    boundary = "replan-" + uuid.uuid4().hex
    metadata = json.dumps({"fileName": file_name})
    safe_name = file_name.replace("\\", "_").replace('"', "_")
    body = b"".join([
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"metadata\"\r\nContent-Type: application/json\r\n\r\n{metadata}\r\n".encode(),
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{safe_name}\"\r\nContent-Type: {content_type}\r\n\r\n".encode(),
        content,
        f"\r\n--{boundary}--\r\n".encode(),
    ])
    return body, f"multipart/form-data; boundary={boundary}"


def command_attachment_upload(args):
    global partial_apply
    command = "openproject wiki attachment upload"
    require_settings(command)
    file_name = args.name or os.path.basename(args.file)
    if mode == "apply":
        partial_apply = helper_envelope.PartialApply()
        partial_apply.attempting({
            "action": "upload_wiki_attachment",
            "page_id": int(args.id) if args.id else None,
            "page_slug": args.slug,
            "page_title": args.title,
            "file_name": file_name,
        })
    page_id = find_page_id(args, command)
    read_page(page_id, command)
    try:
        with open(args.file, "rb") as handle:
            content = handle.read()
    except OSError as exc:
        print_json(envelope(command, mode, errors=[diagnostic("file_unreadable", f"cannot read {args.file}: {exc.strerror}", "error", "file", args.file)]), 2)
    same_name = [a for a in page_attachments(page_id) if a.get("fileName") == file_name]
    action = {"action": "upload_wiki_attachment", "page_id": page_id, "file_name": file_name, "bytes": len(content),
              "sha256": hashlib.sha256(content).hexdigest(), "replaces": [a.get("id") for a in same_name] if args.replace else [],
              "dry_run": mode != "apply"}
    if same_name and not args.replace:
        action["blocked"] = True
        print_json(envelope(command, mode, actions=[action], counts={"planned_actions": 0, "blocked_actions": 1}, errors=[
            diagnostic("attachment_name_taken", f"wiki page {page_id} already has an attachment named {file_name!r} (id {same_name[0].get('id')}); pass --replace to put this file in its place", "error", "attachment", same_name[0].get("id")),
        ]), 2)
    if mode != "apply":
        print_json(envelope(command, "dry_run", actions=[action], counts={"planned_actions": 1, "blocked_actions": 0}, extra={"dry_run": True, "suggested_next_actions": ["Rerun with REPLAN_APPLY=1 to upload."]}))
    body, content_type = multipart(file_name, content, mimetypes.guess_type(file_name)[0] or "application/octet-stream")
    path = f"/api/v3/wiki_pages/{page_id}/attachments"
    req = urllib.request.Request(base_url + path, data=body, method="POST", headers=client.headers(content_type=content_type))
    partial_apply.attempting(action)
    partial_apply.writing()
    _, created = openproject_request_json(
        req, "POST", path, expected=(200, 201), options=settings.request_options()
    )
    uploaded_action = {**action, "dry_run": False, "applied": True, "attachment_id": created.get("id")}
    partial_apply.applied(uploaded_action)
    # The new file is in place before an old one of the same name goes.
    for old in same_name:
        delete_action = {
            "action": "delete_replaced_wiki_attachment",
            "page_id": page_id,
            "attachment_id": old.get("id"),
            "file_name": old.get("fileName", file_name),
        }
        partial_apply.attempting(delete_action)
        partial_apply.writing()
        client.request_json("DELETE", f"/api/v3/attachments/{old.get('id')}", expected=(204,))
        partial_apply.applied({**delete_action, "applied": True})
    action.update({"dry_run": False, "applied": True, "attachment_id": created.get("id")})
    print_json(envelope(command, "apply", actions=[action], items=[attachment_summary(created)], counts={"applied_actions": 1}))


def command_attachment_delete(args):
    command = "openproject wiki attachment delete"
    require_settings(command)
    attachment_id = int(args.attachment)
    try:
        _, attachment = client.request_json("GET", f"/api/v3/attachments/{attachment_id}", expected=(200,))
    except RuntimeError as exc:
        if error_status(exc) == 404:
            print_json(envelope(command, mode, errors=[diagnostic("attachment_not_found", f"attachment {attachment_id} was not found, or this user may not see it", "error", "attachment", attachment_id)]), 2)
        raise
    container = ((attachment.get("_links") or {}).get("container") or {}).get("href", "")
    action = {"action": "delete_wiki_attachment", "attachment_id": attachment_id, "file_name": attachment.get("fileName", ""),
              "page_id": id_from_href(container), "dry_run": mode != "apply"}
    if "/wiki_pages/" not in container:
        action["blocked"] = True
        print_json(envelope(command, mode, actions=[action], counts={"planned_actions": 0, "blocked_actions": 1}, errors=[
            diagnostic("attachment_not_on_wiki_page", f"attachment {attachment_id} does not belong to a wiki page; this helper deletes wiki attachments only", "error", "attachment", attachment_id),
        ]), 2)
    if mode != "apply":
        print_json(envelope(command, "dry_run", actions=[action], counts={"planned_actions": 1, "blocked_actions": 0}, extra={"dry_run": True, "suggested_next_actions": ["Rerun with REPLAN_APPLY=1 to delete the attachment."]}))
    client.request_json("DELETE", f"/api/v3/attachments/{attachment_id}", expected=(204,))
    action.update({"dry_run": False, "applied": True})
    print_json(envelope(command, "apply", actions=[action], counts={"applied_actions": 1}))


def add_page_selector(parser):
    parser.add_argument("--id", default="", help="page id")
    parser.add_argument("--slug", default="", help="page slug, as in the page's address")
    parser.add_argument("--title", default="", help="exact page title")


def main():
    class WikiArgumentParser(helper_envelope.EnvelopeArgumentParser):
        def __init__(self, *args, **kwargs):
            super().__init__("openproject wiki", *args, **kwargs)

    parser = WikiArgumentParser(prog="openproject-wiki.sh", description="Wiki pages and attachments of an OpenProject project. Changes are dry runs unless REPLAN_APPLY=1.", allow_abbrev=False)
    sub = parser.add_subparsers(dest="area", required=True)

    page = sub.add_parser("page", help="wiki pages").add_subparsers(dest="command", required=True)
    page.add_parser("list", help="every page of the project's wiki")
    show = page.add_parser("show", help="one page: facts, attachments and its text")
    add_page_selector(show)
    show.add_argument("--output", default="", help="write the page text to this file instead of the JSON")
    create = page.add_parser("create", help="create a page")
    create.add_argument("--title", required=True)
    create.add_argument("--text-file", default="", help="file holding the page text")
    create.add_argument("--parent-id", default="", help="id of the parent page")
    update = page.add_parser("update", help="replace a page's text or move it under another page")
    add_page_selector(update)
    update.add_argument("--text-file", default="", help="file holding the new page text")
    update.add_argument("--parent-id", default="", help="id of the new parent page, or 'none' for a top-level page")
    update.add_argument("--expect-lock-version", default="", help="refuse unless the page is still at this lock version, the one it was read at")
    update.add_argument("--diff-output", default="", help="write a unified diff of current and new text to this file")
    delete = page.add_parser("delete", help="delete a page with its history and attachments")
    add_page_selector(delete)

    attachment = sub.add_parser("attachment", help="attachments of wiki pages").add_subparsers(dest="command", required=True)
    alist = attachment.add_parser("list", help="attachments of a page")
    add_page_selector(alist)
    download = attachment.add_parser("download", help="download an attachment")
    download.add_argument("--attachment", required=True, help="attachment id")
    download.add_argument("--output", required=True, help="file to write")
    upload = attachment.add_parser("upload", help="attach a file to a page")
    add_page_selector(upload)
    upload.add_argument("--file", required=True)
    upload.add_argument("--name", default="", help="file name on the page; the file's own name by default")
    upload.add_argument("--replace", action="store_true", help="replace an attachment of the same name")
    adelete = attachment.add_parser("delete", help="delete an attachment of a wiki page")
    adelete.add_argument("--attachment", required=True, help="attachment id")

    args = parser.parse_args(argv)
    initialize_environment()
    if args.area == "page" and args.command != "list" and args.command != "create" and not (args.id or args.slug or args.title):
        raise ValueError("one of --id, --slug or --title is required")
    if args.area == "attachment" and args.command in {"list", "upload"} and not (args.id or args.slug or args.title):
        raise ValueError("one of --id, --slug or --title is required")
    handlers = {
        ("page", "list"): command_page_list, ("page", "show"): command_page_show, ("page", "create"): command_page_create,
        ("page", "update"): command_page_update, ("page", "delete"): command_page_delete,
        ("attachment", "list"): command_attachment_list, ("attachment", "download"): command_attachment_download,
        ("attachment", "upload"): command_attachment_upload, ("attachment", "delete"): command_attachment_delete,
    }
    handlers[(args.area, args.command)](args)


try:
    main()
except ValueError as exc:
    print_json(envelope("openproject wiki", mode, ok=False, errors=[diagnostic("invalid_arguments", str(exc))]), 8)
except RuntimeError as exc:
    if error_status(exc) == 403 and not (
        partial_apply is not None and (
            partial_apply.applied_so_far or partial_apply.failed_step is not None
        )
    ):
        # OpenProject does not say which permission; name the ones the wiki uses.
        print_json(envelope("openproject wiki", mode, ok=False, errors=[diagnostic(
            "wiki_permission_denied",
            "OpenProject refused this for the token's user. Reading needs view_wiki_pages; creating, editing and "
            "attachments need edit_wiki_pages; deleting a page and editing a protected page need manage_wiki.")]),
            helper_envelope.EXIT["external"])
    if partial_apply is not None and (
        partial_apply.applied_so_far or partial_apply.failed_step is not None
    ):
        entry, code, partial_extra = partial_apply.failure(exc, "openproject_wiki_error")
    else:
        entry, code = helper_envelope.error_from_exception(exc, "openproject_wiki_error")
        partial_extra = {}
    extra = partial_extra or None
    if partial_apply is not None and partial_apply.applied_so_far:
        extra = {
            **partial_extra,
            "operator_next_steps": [
                "Run attachment upload again as a dry run; replacement lists the current same-name attachments, uploads the new file before deleting old ones, and can resume cleanup without losing the replacement.",
            ],
        }
        if entry.get("outcome_unknown"):
            extra["operator_next_steps"].insert(0, "Check the failed attachment in OpenProject first; that upload or deletion may have been applied.")
    if entry.get("outcome_unknown") and entry.get("method") == "POST":
        if "/attachments" in entry.get("path", ""):
            step = "Check whether the attachment exists before running upload again."
        else:
            step = "Check whether the wiki page exists before running create again."
        if not (partial_apply is not None and partial_apply.applied_so_far):
            extra = {**partial_extra, "operator_next_steps": [step]}
    print_json(envelope("openproject wiki", mode, ok=False, errors=[entry], extra=extra), code)
