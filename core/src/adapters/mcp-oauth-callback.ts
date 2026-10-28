import { randomBytes, timingSafeEqual } from "node:crypto";
import { createServer, type Server } from "node:http";
import { isText } from "../shared/wire.ts";

export class McpOAuthCallback {
  readonly state = randomBytes(32).toString("base64url");
  private server: Server;
  private resolve!: (code: string) => void;
  private reject!: (error: Error) => void;
  private consumed = false;
  private issuer = "";
  private requireIssuer = false;
  private armed = false;
  private port = 0;
  readonly code = new Promise<string>((resolve, reject) => { this.resolve = resolve; this.reject = reject; });

  constructor() {
    void this.code.catch(() => {});
    this.server = createServer((req, res) => {
      res.setHeader("Cache-Control", "no-store");
      res.setHeader("Referrer-Policy", "no-referrer");
      res.setHeader("Content-Type", "text/plain; charset=utf-8");
      res.setHeader("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'");
      const reject = (status: number) => { res.writeHead(status); res.end("OAuth callback rejected."); };
      if (req.method !== "GET" || req.headers.host !== `localhost:${this.port}` || (req.url?.length ?? 0) > 8192) return reject(400);
      let url: URL;
      try { url = new URL(req.url ?? "/", this.redirectUrl); } catch { return reject(400); }
      if (url.origin !== new URL(this.redirectUrl).origin || url.pathname !== "/callback" || url.hash || !this.armed) return reject(400);
      if (this.consumed) return reject(409);
      for (const key of ["state", "code", "iss", "error"]) if (url.searchParams.getAll(key).length > 1) return reject(400);
      const state = url.searchParams.get("state") ?? "";
      const stateBytes = Buffer.from(state), expected = Buffer.from(this.state);
      if (stateBytes.length !== expected.length || !timingSafeEqual(stateBytes, expected)) return reject(400);
      const issuer = url.searchParams.get("iss");
      if (issuer !== null ? issuer !== this.issuer : this.requireIssuer) return reject(400);
      const code = url.searchParams.get("code"), error = url.searchParams.get("error");
      if ((code !== null && error !== null) || (!code && !error) || (code?.length ?? 0) > 4096) return reject(400);
      this.consumed = true;
      res.once("finish", () => {
        if (error) this.reject(new Error("MCP OAuth authorization was denied"));
        else this.resolve(code!);
      });
      res.end(error ? "Authorization was not completed. Return to Neko." : "Authorization response received. Return to Neko to finish connecting.");
    });
    this.server.requestTimeout = 5000;
    this.server.headersTimeout = 5000;
    this.server.maxHeadersCount = 32;
  }

  get redirectUrl(): string { return `http://localhost:${this.port}/callback`; }

  async listen(port: number): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      const error = () => reject(new Error("MCP OAuth callback port is unavailable"));
      this.server.once("error", error);
      this.server.listen(port, "127.0.0.1", () => { this.server.off("error", error); resolve(); });
    });
    const address = this.server.address();
    if (!address || isText(address)) throw new Error("MCP OAuth callback address is unavailable");
    this.port = address.port;
  }

  arm(issuer: string, requireIssuer: boolean): void {
    if (this.armed || this.consumed) throw new Error("MCP OAuth authorization already started");
    this.issuer = issuer;
    this.requireIssuer = requireIssuer;
    this.armed = true;
  }

  cancel(error: Error): void { this.consumed = true; this.reject(error); }

  async close(): Promise<void> {
    this.cancel(new Error("MCP OAuth callback closed"));
    if (!this.server.listening) return;
    await new Promise<void>(resolve => { this.server.close(() => resolve()); this.server.closeAllConnections(); });
  }
}
