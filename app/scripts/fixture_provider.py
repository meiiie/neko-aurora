"""Controlled loopback provider for Core process acceptance; no external requests."""
import argparse
import json
import time
import sys
import threading
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


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
                found = re.search(r'\n(\{"version":[12],.*\})\nCall capabilities', prompt)
                pin = json.loads(found.group(1)) if found else {}
                self.chunk({"role": "assistant", "content": "PIN_SHA="+pin.get("sha256", "missing")+"\nPIN_SOURCES="+json.dumps([s["requestId"] for s in pin.get("sources", [])])})
                self.chunk({}, "stop")
            elif artifact and not has_tool:
                args = {"name": "artifact.write_text", "arguments": {"filename": "bao-cao-neko.md", "text": "# Báo cáo Neko Bot\n\nTệp UTF-8 được ghi qua host sau khi Core yêu cầu công cụ.\n"}}
                self.chunk({"role": "assistant", "tool_calls": [{"index": 0, "id": "fixture-call", "type": "function", "function": {"name": "mcp__nekobot__execute_tool", "arguments": json.dumps(args, ensure_ascii=False)}}]})
                self.chunk({}, "tool_calls")
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
