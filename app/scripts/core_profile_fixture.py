"""Only an owned loopback endpoint for the synthetic Core profile journey."""
import json
from http.server import ThreadingHTTPServer
from e2e_core_profile_bridge import Model


class Handler(Model):
    def do_GET(self):
        if self.path != '/observations':
            self.send_error(404)
            return
        with self.lock:
            raw = json.dumps(self.observations).encode()
        self.send_response(200)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


if __name__ == '__main__':
    server = ThreadingHTTPServer(('127.0.0.1',0),Handler)
    print('READY http://127.0.0.1:%d' % server.server_address[1],flush=True)
    server.serve_forever()
