"""Queue a browser-sized connection burst before the fixture accepts clients."""
import socket
import threading
import unittest
from http.server import BaseHTTPRequestHandler
from fixture_http import FixtureHTTPServer


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):pass
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Length','2')
        self.end_headers()
        self.wfile.write(b'ok')


class FixtureBurst(unittest.TestCase):
    def test_pending_asset_connections_are_accepted_and_served(self):
        server=FixtureHTTPServer(('127.0.0.1',0),Handler)
        clients=[];failures=[];thread=None
        try:
            for index in range(32):
                client=socket.socket();client.settimeout(.25);clients.append(client)
                try:client.connect(server.server_address)
                except OSError as error:failures.append({'connection':index,'errno':error.errno,'type':type(error).__name__})
            self.assertEqual(failures,[],f'Fixture rejected pending asset connections: {failures}')
            thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
            for client in clients:
                client.settimeout(3)
                client.sendall(b'GET /asset HTTP/1.0\r\nHost: localhost\r\n\r\n')
            for client in clients:
                response=b''
                while chunk:=client.recv(4096):response+=chunk
                self.assertTrue(response.startswith(b'HTTP/1.0 200 '),response[:50])
                self.assertTrue(response.endswith(b'\r\n\r\nok'),response[-20:])
        finally:
            for client in clients:client.close()
            if thread:server.shutdown();thread.join(timeout=5)
            server.server_close()


if __name__=='__main__':unittest.main()
