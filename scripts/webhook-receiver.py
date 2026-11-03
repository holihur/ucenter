#!/usr/bin/env python3
"""Tiny local webhook receiver: fails the first request (to exercise retry), then 200."""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LOG = "/tmp/webhook-deliveries.jsonl"
count = 0


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        global count
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8", "replace")
        count += 1
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write(json.dumps({
                "attempt": count,
                "event": self.headers.get("X-UCenter-Event"),
                "delivery": self.headers.get("X-UCenter-Delivery"),
                "timestamp": self.headers.get("X-UCenter-Timestamp"),
                "signature": self.headers.get("X-UCenter-Signature"),
                "body": body,
            }) + "\n")
        status = 500 if count == 1 else 200
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def log_message(self, *_args):
        pass


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", 9092), Handler).serve_forever()
