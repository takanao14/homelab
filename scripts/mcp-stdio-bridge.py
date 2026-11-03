#!/usr/bin/env python3
import json
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
HEADERS_HELPER = ROOT / "scripts/mcp-http-headers.py"


def fail(message):
    print(f"mcp-stdio-bridge: {message}", file=sys.stderr)
    sys.exit(1)


def load_headers(server):
    # The helper output carries the bearer token; keep it in memory only.
    result = subprocess.run(
        [sys.executable, str(HEADERS_HELPER), server],
        capture_output=True, text=True, check=False,
    )
    if result.returncode:
        fail("MCP credential is unavailable")
    try:
        headers = json.loads(result.stdout)
    except ValueError:
        fail("Header helper returned invalid output")
    if not isinstance(headers, dict) or "Authorization" not in headers:
        fail("Header helper returned invalid output")
    return headers


def emit(message):
    sys.stdout.write(message.rstrip("\n") + "\n")
    sys.stdout.flush()


def error_response(request_id, message):
    emit(json.dumps({
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": -32603, "message": message},
    }))


def forward(url, headers, session, line):
    request = urllib.request.Request(url, data=line.encode(), method="POST")
    for name, value in headers.items():
        request.add_header(name, value)
    request.add_header("Content-Type", "application/json")
    request.add_header("Accept", "application/json, text/event-stream")
    if session.get("id"):
        request.add_header("Mcp-Session-Id", session["id"])
    with urllib.request.urlopen(request, timeout=300) as response:
        session_id = response.headers.get("Mcp-Session-Id")
        if session_id:
            session["id"] = session_id
        if response.status == 202:
            return
        if response.headers.get_content_type() != "text/event-stream":
            body = response.read().decode()
            if body.strip():
                emit(body)
            return
        data = []
        for raw in response:
            text = raw.decode().rstrip("\r\n")
            if text.startswith("data:"):
                data.append(text[5:].lstrip(" "))
            elif not text and data:
                emit("\n".join(data))
                data = []
        if data:
            emit("\n".join(data))


def main():
    if len(sys.argv) != 2:
        fail("Usage: mcp-stdio-bridge.py <server>")
    server = sys.argv[1]
    try:
        url = json.loads((ROOT / ".mcp.json").read_text())["mcpServers"][server]["url"]
    except (OSError, ValueError, KeyError, TypeError):
        fail("Server is not defined in .mcp.json")
    headers = load_headers(server)
    session = {}
    for line in sys.stdin:
        if not line.strip():
            continue
        try:
            request_id = json.loads(line).get("id")
        except (ValueError, AttributeError):
            request_id = None
        try:
            forward(url, headers, session, line)
        except urllib.error.HTTPError as error:
            print(f"mcp-stdio-bridge: HTTP {error.code}", file=sys.stderr)
            if request_id is not None:
                error_response(request_id, f"Upstream returned HTTP {error.code}")
        except (urllib.error.URLError, OSError):
            print("mcp-stdio-bridge: upstream request failed", file=sys.stderr)
            if request_id is not None:
                error_response(request_id, "Upstream request failed")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        pass
