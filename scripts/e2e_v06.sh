#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/gitmake"
(cd "$ROOT" && go build -o "$BIN" ./cmd/gitmake)

python3 - "$BIN" "$TMP" <<'PY'
import json, os, subprocess, sys, pathlib
bin_path, tmp = sys.argv[1], pathlib.Path(sys.argv[2])


def start(args, cwd=None):
    return subprocess.Popen([bin_path, *args], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, cwd=cwd, bufsize=1)

def rpc(p, method, params=None, i=1):
    req={"jsonrpc":"2.0","id":i,"method":method}
    if params is not None: req["params"]=params
    p.stdin.write(json.dumps(req)+"\n"); p.stdin.flush()
    line=p.stdout.readline()
    assert line, p.stderr.read()
    return json.loads(line)

# Default MCP must be read-only.
p=start(["mcp"])
r=rpc(p,"tools/list")
names={t["name"] for t in r["result"]["tools"]}
assert "gitmake_preview" in names
assert "gitmake_config_write" not in names
assert "gitmake_apply" not in names

r=rpc(p,"tools/call",{"name":"gitmake_config_schema","arguments":{}},2)
assert not r["result"]["isError"], r
sc=r["result"]["structuredContent"]
assert sc["$id"]=="gitmake.config/v1", sc
p.stdin.close(); p.wait(timeout=5)

# Explicit --allow-write exposes only the gated mutation surface.
proj=tmp/"project"; proj.mkdir()
(proj/"Demo.zip").write_bytes(b"placeholder")
p=start(["mcp","--allow-write"], cwd=proj)
r=rpc(p,"tools/list")
names={t["name"] for t in r["result"]["tools"]}
for name in ["gitmake_config_write","gitmake_config_patch","gitmake_apply"]:
    assert name in names, names

cfg={
  "schema_version":1,
  "repo":{"name":"Demo","visibility":"private"},
  "source":{"zip":"Demo.zip","strip_root":False},
  "git":{"branch":"main"}
}
r=rpc(p,"tools/call",{"name":"gitmake_config_write","arguments":{"config":cfg}},2)
assert not r["result"]["isError"], r
assert (proj/"gitmake.json").exists()
written=r["result"]["structuredContent"]
assert written["written"] is True, written
p.stdin.close(); p.wait(timeout=5)

# Legacy initialize compatibility.
p=start(["mcp"])
r=rpc(p,"initialize",{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}})
assert r["result"]["serverInfo"]["name"]=="gitmake", r
assert r["result"]["serverInfo"]["version"]=="0.6.1", r
p.stdin.close(); p.wait(timeout=5)

print("V06_MCP_E2E_PASS")
PY
