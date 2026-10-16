"""Controlled loopback provider for Core process acceptance; no external requests."""
import argparse
import json
import time
import sys
import threading
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def admitted_fixture_context(messages):
    for message in reversed(messages):
        if message.get("role") != "assistant" or not isinstance(message.get("content"), str):
            continue
        try:
            value = json.loads(message["content"])
        except (ValueError, TypeError):
            continue
        if value.get("kind") == "host_context_manifest" and isinstance(value.get("manifest"), dict):
            return value["manifest"]
    return None


def fixture_pin(prompt, manifest):
    if manifest is None:
        found = re.search(r'\n(\{"version":[123],.*\})\nCall capabilities', prompt)
        return json.loads(found.group(1)) if found else {}
    sources, files = [], []
    for source in manifest.get("sources", []):
        if source.get("kind") == "retrieved":
            files.append({"id": source["id"], "excerpt": source["span"]["text"], "offset": source["span"]["offset"], "bytes": source["span"]["end"] - source["span"]["offset"]})
        elif source.get("kind") == "derived":
            try:
                value = json.loads(source["span"]["text"])
            except ValueError:
                continue
            if value.get("kind") == "pinned-prior-turn":
                sources.append({"requestId": value["requestId"]})
    return {"sha256": manifest["contextPinSHA256"], "sources": sources, "resources": {"files": files}}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        if self.path != "/v1/chat/completions" or self.headers.get("Authorization") != "Bearer fixture-only":
            self.send_error(403)
            return
        length = int(self.headers.get("Content-Length", 0))
        if not 0 < length <= 2_000_000:
            self.send_error(413)
            return
        body = json.loads(self.rfile.read(length))
        messages = body["messages"]
        user = next((m["content"] for m in reversed(messages) if m["role"] == "user"), "")
        if isinstance(user, list):
            user = " ".join(str(v.get("text", "")) for v in user)
        prompt = user
        for message in reversed(messages):
            content = message.get("content", "")
            if message.get("role") == "user" and isinstance(content, str) and "\nCurrent user request:\n" in content:
                prompt = content
                break
        user = prompt.split("\nCurrent user request:\n", 1)[-1]
        manifest = admitted_fixture_context(messages)
        if manifest is not None:
            user = manifest["primaryMessage"]["text"]
        has_tool = any(m["role"] == "tool" for m in messages)
        slow = "[fixture:background]" in user
        artifact = "[fixture:artifact]" in user
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        try:
            if slow and not has_tool:
                self.chunk({"role": "assistant", "content": "Đang xử lý công việc nền… "})
                for _ in range(5):
                    time.sleep(3)
                    self.chunk({"content": "· "})
            if "[fixture:context-inspect]" in user:
                pin = fixture_pin(prompt, manifest)
                resources=pin.get("resources", {})
                self.chunk({"role": "assistant", "content": "PIN_SHA="+pin.get("sha256", "missing")+"\nPIN_SOURCES="+json.dumps([s["requestId"] for s in pin.get("sources", [])])+"\nPIN_FILES="+json.dumps([s["id"] for s in resources.get("files", [])])+"\nPIN_EXCERPTS="+json.dumps([s["excerpt"] for s in resources.get("files", [])])})
                self.chunk({}, "stop")
            elif artifact and not has_tool:
                args = {"name": "artifact.write_text", "arguments": {"filename": "bao-cao-neko.md", "text": "# Báo cáo Neko Bot\n\nTệp UTF-8 được ghi qua host sau khi Core yêu cầu công cụ.\n"}}
                self.chunk({"role": "assistant", "tool_calls": [{"index": 0, "id": "fixture-call", "type": "function", "function": {"name": "mcp__nekobot__execute_tool", "arguments": json.dumps(args, ensure_ascii=False)}}]})
                self.chunk({}, "tool_calls")
            elif "[fixture:file-read]" in user and not has_tool:
                pin=fixture_pin(prompt, manifest)
                files=pin.get("resources",{}).get("files",[])
                source = files[0] if files else {}
                args={"name":"project.read_file","arguments":{"fileId":source.get("id", "missing"),"offset":source.get("offset", 0),"bytes":source.get("bytes", 4000)}}
                self.chunk({"role":"assistant","tool_calls":[{"index":0,"id":"fixture-read-call","type":"function","function":{"name":"mcp__nekobot__execute_tool","arguments":json.dumps(args)}}]})
                self.chunk({},"tool_calls")
            elif "[fixture:capabilities]" in user and not has_tool:
                self.chunk({"role":"assistant","tool_calls":[{"index":0,"id":"fixture-capability-call","type":"function","function":{"name":"mcp__nekobot__capabilities","arguments":"{}"}}]})
                self.chunk({},"tool_calls")
            elif any(marker in user for marker in ["[fixture:file-read]","[fixture:capabilities]"]):
                tool=next((m.get("content","") for m in reversed(messages) if m["role"]=="tool"),"")
                self.chunk({"role":"assistant","content":"READ_RESULT="+str(tool)})
                self.chunk({},"stop")
            else:
                for text in ["Core đã nhận tin. ", "Công việc nền vẫn tiếp tục." if not slow else "Báo cáo đã hoàn thành."]:
                    self.chunk({"role": "assistant", "content": text})
                    time.sleep(.15)
                self.chunk({}, "stop")
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def chunk(self, delta, finish=None):
        payload = {"id": "controlled-fixture", "object": "chat.completion.chunk", "created": 1,
                   "model": "neko-controlled-fixture", "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
        self.wfile.write(("data: " + json.dumps(payload, ensure_ascii=False) + "\n\n").encode())
        self.wfile.flush()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=0)
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    def lifeline():
        sys.stdin.buffer.read()
        server.shutdown()
    threading.Thread(target=lifeline, daemon=True).start()
    print("READY http://127.0.0.1:%d/v1" % server.server_port, flush=True)
    server.serve_forever()
