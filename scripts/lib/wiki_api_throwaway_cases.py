#!/usr/bin/env python3
"""HTTP cases for the OpenProject wiki API plugin throwaway runtime check.

Real HTTP calls with API tokens against a disposable OpenProject. Every case
records the request, the status code and the asserted facts, so the evidence
can be re-run and checked by someone who did not write it.

Usage: wiki_api_throwaway_cases.py <base_url> <seed.json> <evidence.json>
"""

import base64
import json
import sys
import urllib.error
import urllib.request
import uuid

BASE, SEED_PATH, EVIDENCE_PATH = sys.argv[1].rstrip("/"), sys.argv[2], sys.argv[3]
SEED = json.load(open(SEED_PATH))
TOKENS, PROJECTS, PAGES = SEED["tokens"], SEED["projects"], SEED["pages"]

results = []


def call(actor, method, path, body=None, raw=None, content_type="application/json"):
    data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
    request = urllib.request.Request(BASE + path, data=data, method=method)
    auth = base64.b64encode(f"apikey:{TOKENS[actor]}".encode()).decode()
    request.add_header("Authorization", f"Basic {auth}")
    if data is not None:
        request.add_header("Content-Type", content_type)
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            status, payload = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, payload = error.code, error.read()
    try:
        parsed = json.loads(payload) if payload else {}
    except ValueError:
        parsed = {"_unparsed": payload[:200].decode("utf-8", "replace")}
    return status, parsed


def case(number, title):
    def wrap(function):
        record = {"case": number, "title": title, "requests": [], "checks": [], "ok": True}

        def request(actor, method, path, body=None, **kwargs):
            status, parsed = call(actor, method, path, body, **kwargs)
            record["requests"].append({"actor": actor, "method": method, "path": path, "status": status})
            return status, parsed

        def check(description, condition, observed=None):
            passed = bool(condition)
            record["checks"].append({"check": description, "ok": passed, "observed": observed})
            if not passed:
                record["ok"] = False

        try:
            function(request, check)
        except Exception as error:  # a crashed case is a failed case, not a crashed run
            record["ok"] = False
            record["checks"].append({"check": "case ran without exception", "ok": False, "observed": repr(error)})
        results.append(record)
        print(f"case {number:>2} {'ok  ' if record['ok'] else 'FAIL'} {title}")
        return function

    return wrap


def page_path(page_id):
    return f"/api/v3/wiki_pages/{page_id}"


def collection_path(project):
    return f"/api/v3/projects/{PROJECTS[project]}/wiki_pages"


state = {}


@case(1, "editor lists pages; elements carry no text")
def _(request, check):
    status, body = request("editor", "GET", collection_path("main"))
    elements = body.get("_embedded", {}).get("elements", [])
    check("status 200", status == 200, status)
    check("collection type", body.get("_type") == "Collection", body.get("_type"))
    check("seeded pages listed", {"Start", "Protected"} <= {e.get("title") for e in elements},
          [e.get("title") for e in elements])
    check("no element has text", all("text" not in e for e in elements))
    check("elements carry slug and lockVersion", all("slug" in e and "lockVersion" in e for e in elements))
    titles = [e.get("title") for e in elements]
    check("ordered by title", titles == sorted(titles), titles)


@case(2, "editor reads one page with text.raw")
def _(request, check):
    status, body = request("editor", "GET", page_path(PAGES["start"]))
    check("status 200", status == 200, status)
    check("text.raw present", "Seeded start page." in (body.get("text") or {}).get("raw", ""), body.get("text"))
    check("text.format markdown", (body.get("text") or {}).get("format") == "markdown")
    check("lockVersion present", isinstance(body.get("lockVersion"), int), body.get("lockVersion"))
    check("slug present", body.get("slug") == "start", body.get("slug"))
    links = body.get("_links", {})
    check("update link for editor", links.get("update", {}).get("method") == "patch", links.get("update"))
    check("upstream attachment links kept", "attachments" in links and "addAttachment" in links, sorted(links))
    check("project link kept", "project" in links)


@case(3, "editor creates a page; GET returns the text")
def _(request, check):
    title = f"Created {uuid.uuid4().hex[:8]}"
    status, body = request("editor", "POST", collection_path("main"),
                           {"title": title, "text": {"raw": "created by token"}})
    check("status 201", status == 201, status)
    check("title echoed", body.get("title") == title, body.get("title"))
    state["created_id"] = body.get("id")
    status, body = request("editor", "GET", page_path(state["created_id"]))
    check("follow-up GET 200", status == 200, status)
    check("text persisted", (body.get("text") or {}).get("raw") == "created by token", body.get("text"))
    state["created_lock"] = body.get("lockVersion")
    state["created_title"] = title


@case(4, "PATCH with current lockVersion changes text and increments lockVersion")
def _(request, check):
    status, body = request("editor", "PATCH", page_path(state["created_id"]),
                           {"lockVersion": state["created_lock"], "text": {"raw": "edited by token"}})
    check("status 200", status == 200, status)
    check("text changed in response", (body.get("text") or {}).get("raw") == "edited by token", body.get("text"))
    check("lockVersion incremented", body.get("lockVersion", -1) > state["created_lock"],
          [state["created_lock"], body.get("lockVersion")])
    status, body = request("editor", "GET", page_path(state["created_id"]))
    check("GET after PATCH shows new text (cache invalidated)",
          (body.get("text") or {}).get("raw") == "edited by token", body.get("text"))
    state["stale_lock"] = state["created_lock"]
    state["created_lock"] = body.get("lockVersion")


@case(5, "PATCH without lockVersion is 409 and leaves text unchanged")
def _(request, check):
    status, body = request("editor", "PATCH", page_path(state["created_id"]), {"text": {"raw": "lost update"}})
    check("status 409", status == 409, status)
    check("UpdateConflict identifier", "UpdateConflict" in body.get("errorIdentifier", ""), body.get("errorIdentifier"))
    _, body = request("editor", "GET", page_path(state["created_id"]))
    check("text unchanged", (body.get("text") or {}).get("raw") == "edited by token", body.get("text"))


@case(6, "PATCH with stale lockVersion is 409 and leaves text unchanged")
def _(request, check):
    status, body = request("editor", "PATCH", page_path(state["created_id"]),
                           {"lockVersion": state["stale_lock"], "text": {"raw": "stale write"}})
    check("status 409", status == 409, status)
    check("UpdateConflict identifier", "UpdateConflict" in body.get("errorIdentifier", ""), body.get("errorIdentifier"))
    _, body = request("editor", "GET", page_path(state["created_id"]))
    check("text unchanged", (body.get("text") or {}).get("raw") == "edited by token", body.get("text"))


@case(7, "PATCH with title is 422 and does not rename")
def _(request, check):
    status, body = request("editor", "PATCH", page_path(state["created_id"]),
                           {"lockVersion": state["created_lock"], "title": "Renamed by token"})
    check("status 422", status == 422, status)
    check("PropertyIsReadOnly identifier", "PropertyIsReadOnly" in body.get("errorIdentifier", ""),
          body.get("errorIdentifier"))
    _, body = request("editor", "GET", page_path(state["created_id"]))
    check("title unchanged", body.get("title") == state["created_title"], body.get("title"))
    status, body = request("editor", "PATCH", page_path(state["created_id"]),
                           {"lockVersion": state["created_lock"], "protected": True})
    check("protected in payload is 422", status == 422, status)


@case(8, "editor without manage_wiki cannot PATCH a protected page")
def _(request, check):
    _, body = request("editor", "GET", page_path(PAGES["protected"]))
    check("page is protected", body.get("protected") is True, body.get("protected"))
    status, _ = request("editor", "PATCH", page_path(PAGES["protected"]),
                        {"lockVersion": body.get("lockVersion"), "text": {"raw": "should not land"}})
    check("status 403", status == 403, status)
    _, after = request("editor", "GET", page_path(PAGES["protected"]))
    check("text unchanged", (after.get("text") or {}).get("raw") == "Protected page.", after.get("text"))


@case(9, "read-only member lists and reads; POST and PATCH are 403")
def _(request, check):
    status, _ = request("reader", "GET", collection_path("main"))
    check("list 200", status == 200, status)
    status, body = request("reader", "GET", page_path(PAGES["start"]))
    check("read 200", status == 200, status)
    check("no update link for read-only member", "update" not in body.get("_links", {}),
          sorted(body.get("_links", {})))
    status, _ = request("reader", "POST", collection_path("main"), {"title": "Reader page"})
    check("POST 403", status == 403, status)
    status, _ = request("reader", "PATCH", page_path(PAGES["start"]),
                        {"lockVersion": body.get("lockVersion"), "text": {"raw": "reader write"}})
    check("PATCH 403", status == 403, status)
    status, again = request("editor", "GET", page_path(PAGES["start"]))
    check("editor still gets update link after reader's cached read", "update" in again.get("_links", {}),
          sorted(again.get("_links", {})))


@case(10, "token without membership gets 404 for the page")
def _(request, check):
    status, _ = request("outsider", "GET", page_path(PAGES["start"]))
    check("status 404", status == 404, status)
    status, _ = request("outsider", "PATCH", page_path(PAGES["start"]), {"lockVersion": 0, "text": {"raw": "x"}})
    check("PATCH also 404", status == 404, status)


@case(11, "project without a wiki: collection GET and POST are 404")
def _(request, check):
    for project in ("module_off", "no_record"):
        status, _ = request("editor", "GET", collection_path(project))
        check(f"{project}: GET 404", status == 404, status)
        status, _ = request("editor", "POST", collection_path(project), {"title": "Must not exist"})
        check(f"{project}: POST 404", status == 404, status)


@case(12, "attachments on the same wiki page id still work")
def _(request, check):
    path = page_path(state["created_id"]) + "/attachments"
    status, body = request("editor", "GET", path)
    check("GET attachments 200", status == 200, status)
    boundary = uuid.uuid4().hex
    metadata = json.dumps({"fileName": "throwaway.txt"})
    raw = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"metadata\"\r\n\r\n{metadata}\r\n"
           f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"throwaway.txt\"\r\n"
           f"Content-Type: text/plain\r\n\r\nthrowaway attachment\r\n--{boundary}--\r\n").encode()
    status, body = request("editor", "POST", path, raw=raw,
                           content_type=f"multipart/form-data; boundary={boundary}")
    check("POST attachment 201", status == 201, [status, body.get("message")])
    status, body = request("editor", "GET", path)
    check("attachment listed", body.get("total") == 1, body.get("total"))


@case(13, "editor edits a page written by someone else (journal checked in the database step)")
def _(request, check):
    # OpenProject aggregates consecutive edits of one user into one journal, in
    # the browser as well. A separate history entry therefore needs a second
    # author: the seeded start page was written by the admin.
    _, body = request("editor", "GET", page_path(PAGES["start"]))
    status, body = request("editor", "PATCH", page_path(PAGES["start"]),
                           {"lockVersion": body.get("lockVersion"),
                            "text": {"raw": "# Start\n\nEdited by the token user."}})
    check("status 200", status == 200, status)
    check("text changed", "Edited by the token user." in (body.get("text") or {}).get("raw", ""), body.get("text"))
    state["journal_page_id"] = PAGES["start"]


@case(16, "child page through _links.parent; cross-project parent is 422")
def _(request, check):
    parent_href = page_path(PAGES["start"])
    status, body = request("editor", "POST", collection_path("main"),
                           {"title": f"Child {uuid.uuid4().hex[:8]}", "text": {"raw": "child"},
                            "_links": {"parent": {"href": parent_href}}})
    check("POST child 201", status == 201, [status, body.get("message")])
    child_id = body.get("id")
    _, body = request("editor", "GET", page_path(child_id))
    check("parent link shown", body.get("_links", {}).get("parent", {}).get("href") == parent_href,
          body.get("_links", {}).get("parent"))
    status, error = request("editor", "PATCH", page_path(child_id),
                            {"lockVersion": body.get("lockVersion"),
                             "_links": {"parent": {"href": page_path(PAGES["foreign"])}}})
    check("cross-project parent 422", status == 422, [status, error.get("message")])
    _, body = request("editor", "GET", page_path(child_id))
    check("parent unchanged", body.get("_links", {}).get("parent", {}).get("href") == parent_href,
          body.get("_links", {}).get("parent"))
    state["child_id"] = child_id


@case(17, "member without view_wiki_pages gets 403 for the collection")
def _(request, check):
    status, _ = request("nowiki", "GET", collection_path("main"))
    check("status 403", status == 403, status)


@case(18, "delete link and 403 follow manage_wiki; editor, reader and outsider cannot delete")
def _(request, check):
    _, body = request("editor", "GET", page_path(state["created_id"]))
    check("no delete link for editor without manage_wiki", "delete" not in body.get("_links", {}),
          sorted(body.get("_links", {})))
    _, body = request("manager", "GET", page_path(state["created_id"]))
    link = body.get("_links", {}).get("delete", {})
    check("delete link for manager", link.get("method") == "delete" and link.get("href") == page_path(state["created_id"]),
          link)
    status, _ = request("editor", "DELETE", page_path(state["created_id"]))
    check("editor DELETE 403", status == 403, status)
    status, _ = request("reader", "DELETE", page_path(state["created_id"]))
    check("reader DELETE 403", status == 403, status)
    status, _ = request("outsider", "DELETE", page_path(state["created_id"]))
    check("outsider DELETE 404", status == 404, status)
    status, _ = request("editor", "GET", page_path(state["created_id"]))
    check("page still there", status == 200, status)


@case(19, "DELETE of a page with child pages is 422 and deletes nothing")
def _(request, check):
    status, body = request("manager", "DELETE", page_path(PAGES["start"]))
    check("status 422", status == 422, [status, body.get("message")])
    status, _ = request("editor", "GET", page_path(PAGES["start"]))
    check("parent still there", status == 200, status)
    status, _ = request("editor", "GET", page_path(state["child_id"]))
    check("child still there", status == 200, status)


@case(20, "create then delete: 204, then 404 on GET and on a second DELETE")
def _(request, check):
    title = f"Scratch {uuid.uuid4().hex[:8]}"
    status, body = request("manager", "POST", collection_path("main"), {"title": title, "text": {"raw": "scratch"}})
    check("POST 201", status == 201, status)
    scratch_id = body.get("id")
    status, body = request("manager", "DELETE", page_path(scratch_id))
    check("DELETE 204", status == 204, [status, body])
    status, _ = request("manager", "GET", page_path(scratch_id))
    check("GET after DELETE 404", status == 404, status)
    _, body = request("editor", "GET", collection_path("main"))
    check("not listed any more", title not in {e.get("title") for e in body.get("_embedded", {}).get("elements", [])})
    status, _ = request("manager", "DELETE", page_path(scratch_id))
    check("second DELETE 404", status == 404, status)
    state["deleted_id"] = scratch_id


@case(21, "a leaf child page with an attachment and a protected page can be deleted by manage_wiki")
def _(request, check):
    path = page_path(state["child_id"]) + "/attachments"
    boundary = uuid.uuid4().hex
    metadata = json.dumps({"fileName": "child.txt"})
    raw = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"metadata\"\r\n\r\n{metadata}\r\n"
           f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"child.txt\"\r\n"
           f"Content-Type: text/plain\r\n\r\nchild attachment\r\n--{boundary}--\r\n").encode()
    status, body = request("editor", "POST", path, raw=raw, content_type=f"multipart/form-data; boundary={boundary}")
    check("attachment on child 201", status == 201, [status, body.get("message")])
    state["child_attachment_id"] = body.get("id")
    status, body = request("manager", "DELETE", page_path(state["child_id"]))
    check("child DELETE 204", status == 204, [status, body])
    status, _ = request("manager", "GET", page_path(state["child_id"]))
    check("child GET 404", status == 404, status)
    status, _ = request("editor", "GET", page_path(PAGES["start"]))
    check("parent kept", status == 200, status)
    status, body = request("manager", "DELETE", page_path(PAGES["protected"]))
    check("protected page DELETE 204 with manage_wiki", status == 204, [status, body])


failed = [record["case"] for record in results if not record["ok"]]
json.dump({"ok": not failed,
           "failed_cases": failed,
           "created_page_id": state.get("created_id"),
           "journal_page_id": state.get("journal_page_id"),
           "deleted_page_ids": [state.get("deleted_id"), state.get("child_id"), PAGES["protected"]],
           "deleted_attachment_id": state.get("child_attachment_id"),
           "results": results},
          open(EVIDENCE_PATH, "w"), indent=2, ensure_ascii=False)
print(f"http cases: {len(results) - len(failed)} of {len(results)} ok; failed: {failed}")
sys.exit(1 if failed else 0)
