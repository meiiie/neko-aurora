"""Loopback HTTP server shared by browser fixtures."""
from http.server import ThreadingHTTPServer

class FixtureHTTPServer(ThreadingHTTPServer):
    request_queue_size = 64
