"""Loopback-only finite collaboration fixture. Attempts forbidden writes deliberately."""
import io
import json
import re
import sys
import threading
from http.server import ThreadingHTTPServer

from fixture_provider import Handler as BaseHandler, admitted_fixture_context, fixture_pin


class Handler(BaseHandler):
    slow_gate = threading.Event()

    def do_POST(self):
        if self.path != "/v1/chat/completions" or self.headers.get("Authorization") != "Bearer fixture-only":
            self.send_error(403)
            return
        length = int(self.headers.get("Content-Length", 0))
        if not 0 < length <= 2_000_000:
            self.send_error(413)
            return
        raw = self.rfile.read(length)
        body = json.loads(raw)
        messages = body["messages"]
        prompt = next((m["content"] for m in reversed(messages) if m["role"] == "user" and isinstance(m.get("content"), str) and "\nCurrent user request:\n" in m["content"]), "")
        user = prompt.split("\nCurrent user request:\n", 1)[-1]
        manifest = admitted_fixture_context(messages)
        stage_text = user
        if manifest is not None:
            user = manifest["primaryMessage"]["text"]
            stage_text = ""
            for source in manifest["sources"]:
                if source["kind"] != "derived": continue
                try: value = json.loads(source["span"]["text"])
                except ValueError: continue
                if value.get("kind") == "generated-collaboration-stage":
                    stage_text = value.get("text", "")
                    break
        match = re.search(r"Finite collaboration (collab-[0-9a-f]+), stage ([123]) of 3", stage_text)
        if not match:
            self.rfile = io.BytesIO(raw)
            return super().do_POST()
        group, stage = match.group(1), int(match.group(2))
        tool = next((m.get("content", "") for m in reversed(messages) if m["role"] == "tool"), None)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        try:
            if tool is None:
                if "[collaboration:slow]" in user:
                    self.chunk({"role": "assistant", "content": f"STAGE_{stage}_RUNNING "})
                    # A held stage proves concurrent work independent of machine startup speed.
                    self.slow_gate.wait(60)
                args = {"name": "artifact.write_text", "arguments": {"filename": "collaboration-final.md", "text": f"# Finite conclusion\nGroup {group}; final stage {stage}.\n"}}
                self.chunk({"role": "assistant", "tool_calls": [{"index": 0, "id": f"collab-{stage}", "type": "function", "function": {"name": "mcp__nekobot__execute_tool", "arguments": json.dumps(args)}}]})
                self.chunk({}, "tool_calls")
            else:
                pin = fixture_pin(prompt, manifest)
                result = f"STAGE={stage}\nPIN_SOURCES=" + json.dumps([s["requestId"] for s in pin.get("sources", [])]) + "\nWRITE_RESULT=" + str(tool)
                self.chunk({"role": "assistant", "content": result})
                self.chunk({}, "stop")
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


if __name__ == "__main__":
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    def lifeline():
        sys.stdin.buffer.read()
        Handler.slow_gate.set()
        server.shutdown()
    threading.Thread(target=lifeline, daemon=True).start()
    print("READY http://127.0.0.1:%d/v1" % server.server_port, flush=True)
    server.serve_forever()
