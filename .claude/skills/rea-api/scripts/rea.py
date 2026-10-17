#!/usr/bin/env python3
"""Command-line client for a self-hosted REA Importify server (Python 3.9+, standard library only).

Configuration (environment):
  REA_URL    server URL, e.g. https://rea.example.com or http://localhost:8080
  REA_TOKEN  personal API token (reap_...) from Profile -> API tokens in the dashboard

Every command prints JSON (or Markdown for `report`). Exit status is non-zero on API errors.
"""
import argparse
import json
import mimetypes
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


class APIError(Exception):
    def __init__(self, status, body):
        self.status = status
        self.body = body
        err = body.get("error", {}) if isinstance(body, dict) else {}
        super().__init__(f"HTTP {status} {err.get('code', '')}: {err.get('message', body)}")


def config():
    url = os.environ.get("REA_URL", "").rstrip("/")
    token = os.environ.get("REA_TOKEN", "")
    if not url or not token:
        sys.exit("Set REA_URL and REA_TOKEN (a personal API token from Profile -> API tokens).")
    if not token.startswith("reap_"):
        sys.exit("REA_TOKEN must be a personal API token starting with reap_.")
    return url, token


def request(method, path, body=None, query=None, form=None, raw=False):
    url, token = config()
    full = f"{url}/api/v1{path}"
    if query:
        full += "?" + urllib.parse.urlencode({k: v for k, v in query.items() if v not in (None, "")}, doseq=True)
    headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    data = None
    if form is not None:
        boundary = uuid.uuid4().hex
        parts = []
        for name, value in form.items():
            if isinstance(value, tuple):
                filename, content = value
                ctype = mimetypes.guess_type(filename)[0] or "application/octet-stream"
                parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"; filename="{os.path.basename(filename)}"\r\nContent-Type: {ctype}\r\n\r\n'.encode() + content + b"\r\n")
            else:
                parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
        data = b"".join(parts) + f"--{boundary}--\r\n".encode()
        headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"
    elif body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(full, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            payload = resp.read()
            if raw:
                return payload
            if resp.status == 204 or not payload:
                return None
            return json.loads(payload)
    except urllib.error.HTTPError as e:
        text = e.read()
        try:
            parsed = json.loads(text)
        except ValueError:
            parsed = text.decode(errors="replace")
        raise APIError(e.code, parsed) from None


def out(value):
    print(json.dumps(value, indent=2, ensure_ascii=False))


def read_file(path):
    with open(path, "rb") as f:
        return f.read()


# ---- commands ----

def cmd_whoami(a):
    me = request("GET", "/auth/me")
    out({"user": me["user"]["email"], "organization": me["organization"], "permissions": me["permissions"]})


def cmd_search(a):
    out(request("GET", "/search", query={"q": a.query})["items"])


def cmd_get(a):
    out(request("GET", a.path))


def cmd_post(a):
    out(request("POST", a.path, body=json.loads(read_file(a.json_file)) if a.json_file else {}))


def cmd_projects(a):
    out(request("GET", "/analysis/projects", query={"q": a.q, "limit": a.limit})["items"])


def cmd_project_create(a):
    if not a.i_am_authorized:
        sys.exit("Refusing: pass --i-am-authorized only if you own or are authorized to analyze every source you will add.")
    workspace = a.workspace
    if not workspace:
        ws = request("GET", "/workspaces")["items"]
        if not ws:
            sys.exit("No workspace exists; create one in the dashboard first.")
        workspace = ws[0]["id"]
    body = {
        "name": a.name, "description": a.description, "objective": a.objective, "workspace_id": workspace,
        "analysis_types": [t for t in a.types.split(",") if t], "status": "active", "owner_id": None,
        "authorization_confirmed": True, "authorization_basis": a.basis, "authorization_note": a.note,
    }
    out(request("POST", "/analysis/projects", body=body))


def cmd_source_add(a):
    body = {"kind": a.kind, "title": a.title, "content": "", "url": None, "path": "", "steps": [], "fetch": False, "captured_page_id": None, "auto_analyze": not a.no_analyze}
    if a.kind == "url":
        body["url"] = a.url
        body["fetch"] = a.fetch
    elif a.kind == "repository_path":
        body["path"] = a.path
    elif a.kind == "captured_page":
        body["captured_page_id"] = a.captured_page
    elif a.kind == "workflow":
        body["steps"] = json.loads(read_file(a.steps_file))
    else:
        body["content"] = read_file(a.content_file).decode() if a.content_file else sys.stdin.read()
    out(request("POST", f"/analysis/projects/{a.project}/sources", body=body))


def cmd_source_upload(a):
    form = {"kind": a.kind, "title": a.title or os.path.basename(a.file), "auto_analyze": "false" if a.no_analyze else "true", "file": (a.file, read_file(a.file))}
    out(request("POST", f"/analysis/projects/{a.project}/sources/upload", form=form))


def runs(project):
    return request("GET", f"/analysis/projects/{project}/runs")["items"]


def cmd_runs(a):
    deadline = time.time() + a.timeout
    while True:
        items = runs(a.project)
        active = [r for r in items if r["status"] in ("queued", "running")]
        if not a.wait or not active:
            out([{k: r.get(k) for k in ("id", "analyzer", "status", "error", "source_title", "finished_at")} for r in items])
            if any(r["status"] == "failed" for r in items):
                sys.exit(2)
            return
        if time.time() > deadline:
            sys.exit(f"Timed out with {len(active)} runs still active.")
        time.sleep(2)


def cmd_findings(a):
    q = {"classification": a.classification, "category": a.category, "q": a.q, "limit": a.limit}
    items = request("GET", f"/analysis/projects/{a.project}/findings", query=q)["items"]
    if a.full:
        out(items)
    else:
        out([{k: f.get(k) for k in ("id", "classification", "confidence", "category", "title", "evidence_locator", "source_title")} for f in items])


def cmd_report(a):
    rep = request("POST", f"/analysis/projects/{a.project}/reports", body={"kind": a.kind, "title": a.title, "content_markdown": ""})
    md = request("GET", f"/analysis/reports/{rep['id']}")["report"]["content_markdown"]
    if a.out:
        with open(a.out, "w", encoding="utf-8") as f:
            f.write(md)
        out({"report_id": rep["id"], "written": a.out})
    else:
        print(md)


def cmd_product_check(a):
    out(request("POST", "/imports/products/check", body=import_body(a)))


def cmd_product_import(a):
    out(request("POST", "/imports/products", body=import_body(a)))


def import_body(a):
    payload = json.loads(read_file(a.payload))
    return {"payload": payload, "source": "api", "decision": getattr(a, "decision", "") or "", "target_product_id": getattr(a, "target", None),
            "supplier_id": None, "supplier_contact_id": None, "create_supplier": bool(getattr(a, "create_supplier", False)), "tag_names": [], "captured_page_id": None}


def main():
    p = argparse.ArgumentParser(prog="rea", description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("whoami").set_defaults(fn=cmd_whoami)
    s = sub.add_parser("search"); s.add_argument("query"); s.set_defaults(fn=cmd_search)
    s = sub.add_parser("get", help="GET any API path, e.g. /companies?q=acme"); s.add_argument("path"); s.set_defaults(fn=cmd_get)
    s = sub.add_parser("post", help="POST a JSON file to any API path"); s.add_argument("path"); s.add_argument("json_file", nargs="?"); s.set_defaults(fn=cmd_post)
    s = sub.add_parser("projects"); s.add_argument("--q"); s.add_argument("--limit", type=int, default=50); s.set_defaults(fn=cmd_projects)
    s = sub.add_parser("project-create")
    s.add_argument("--name", required=True); s.add_argument("--description", default=""); s.add_argument("--objective", default="")
    s.add_argument("--types", default="", help="comma list: website,product,codebase,api,document,workflow,reconstruction")
    s.add_argument("--basis", required=True, choices=["owned", "client_authorized", "public_information", "licensed", "open_source", "other"])
    s.add_argument("--note", default=""); s.add_argument("--workspace"); s.add_argument("--i-am-authorized", action="store_true")
    s.set_defaults(fn=cmd_project_create)
    s = sub.add_parser("source-add")
    s.add_argument("project"); s.add_argument("--kind", required=True, choices=["text", "html", "openapi", "api_examples", "url", "workflow", "repository_path", "captured_page"])
    s.add_argument("--title", required=True); s.add_argument("--content-file"); s.add_argument("--url"); s.add_argument("--fetch", action="store_true")
    s.add_argument("--path"); s.add_argument("--captured-page"); s.add_argument("--steps-file"); s.add_argument("--no-analyze", action="store_true")
    s.set_defaults(fn=cmd_source_add)
    s = sub.add_parser("source-upload")
    s.add_argument("project"); s.add_argument("--kind", required=True, choices=["repository_archive", "document", "screenshot", "openapi", "api_examples", "html"])
    s.add_argument("file"); s.add_argument("--title"); s.add_argument("--no-analyze", action="store_true"); s.set_defaults(fn=cmd_source_upload)
    s = sub.add_parser("runs"); s.add_argument("project"); s.add_argument("--wait", action="store_true"); s.add_argument("--timeout", type=int, default=600); s.set_defaults(fn=cmd_runs)
    s = sub.add_parser("findings"); s.add_argument("project"); s.add_argument("--classification", choices=["verified", "inferred", "unknown"])
    s.add_argument("--category"); s.add_argument("--q"); s.add_argument("--limit", type=int, default=200); s.add_argument("--full", action="store_true"); s.set_defaults(fn=cmd_findings)
    s = sub.add_parser("report"); s.add_argument("project")
    s.add_argument("--kind", required=True, choices=["summary", "technical_documentation", "architecture", "api_inventory", "data_model", "workflow", "reconstruction_plan"])
    s.add_argument("--title", default=""); s.add_argument("--out"); s.set_defaults(fn=cmd_report)
    s = sub.add_parser("product-check"); s.add_argument("payload"); s.set_defaults(fn=cmd_product_check)
    s = sub.add_parser("product-import"); s.add_argument("payload"); s.add_argument("--decision", choices=["create_new", "update_existing", "skip"], default="")
    s.add_argument("--target"); s.add_argument("--create-supplier", action="store_true"); s.set_defaults(fn=cmd_product_import)
    a = p.parse_args()
    try:
        a.fn(a)
    except APIError as e:
        print(str(e), file=sys.stderr)
        if isinstance(e.body, dict):
            print(json.dumps(e.body, indent=2), file=sys.stderr)
        sys.exit(1)
    except urllib.error.URLError as e:
        sys.exit(f"Cannot reach REA at {os.environ.get('REA_URL')}: {e.reason}")


if __name__ == "__main__":
    main()
