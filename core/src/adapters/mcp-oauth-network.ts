import { request as httpsRequest } from "node:https";
import type { LookupFunction } from "node:net";
import type { FetchLike } from "@modelcontextprotocol/sdk/shared/transport.js";
import { throwIfAborted } from "../shared/abort.ts";
import { isText } from "../shared/wire.ts";
import { readBoundedBody, resolvePublicHttpAddresses, type PublicLookup } from "./public-http.ts";

type OAuthNetworkDependencies = { lookup?: PublicLookup; request?: typeof httpsRequest };

/** OAuth discovery and credential exchange only: one pinned HTTPS attempt, never an RPC/SSE stream. */
export async function oauthHttpFetch(input: Parameters<FetchLike>[0], init: RequestInit = {}, dependencies: OAuthNetworkDependencies = {}): Promise<Response> {
  try {
    const url = new URL(String(input));
    if (url.protocol !== "https:" || url.username || url.password || url.hash) throw new Error("invalid endpoint");
    const method = (init.method ?? "GET").toUpperCase();
    if (method !== "GET" && method !== "POST") throw new Error("invalid method");
    if (init.body != null && !(isText(init.body) || init.body instanceof URLSearchParams)) throw new Error("invalid body");
    const body = init.body == null ? undefined : Buffer.from(String(init.body));
    if ((method === "GET" && body) || (body && body.byteLength > 64 * 1024)) throw new Error("invalid body");
    const signal = AbortSignal.any([AbortSignal.timeout(30_000), ...(init.signal ? [init.signal] : [])]);
    const [address] = await resolvePublicHttpAddresses(url.href, { lookup: dependencies.lookup, signal });
    if (!address) throw new Error("missing address");
    throwIfAborted(signal);
    const headers = new Headers(init.headers);
    for (const name of ["cookie", "proxy-authorization", "host", "content-length", "transfer-encoding", "connection"]) headers.delete(name);
    headers.set("accept-encoding", "identity");
    if (body) headers.set("content-length", String(body.byteLength));
    const lookup: LookupFunction = (_hostname, options, callback) => {
      if (options.all) callback(null, [address]);
      else callback(null, address.address, address.family);
    };
    return await new Promise<Response>((resolve, reject) => {
      const request = (dependencies.request ?? httpsRequest)(url, {
        method, headers: Object.fromEntries(headers), agent: false, lookup,
        family: address.family, rejectUnauthorized: true,
        maxHeaderSize: 32 * 1024, signal,
      }, async response => {
        try {
          const status = response.statusCode ?? 0;
          if (status < 200 || status >= 600 || (status >= 300 && status < 400)) throw new Error("invalid status");
          const limit = method === "POST" || status >= 400 ? 64 * 1024 : 1024 * 1024;
          const length = response.headers["content-length"];
          if (length && (!/^\d+$/.test(length) || Number(length) > limit)) throw new Error("oversized response");
          const encoding = response.headers["content-encoding"];
          if (encoding && encoding !== "identity") throw new Error("unsupported encoding");
          const data = await readBoundedBody(response, limit);
          throwIfAborted(signal);
          const responseHeaders = new Headers();
          for (const [key, value] of Object.entries(response.headers)) {
            if (Array.isArray(value)) for (const item of value) responseHeaders.append(key, item);
            else if (value !== undefined) responseHeaders.set(key, value);
          }
          const result = new Response(status === 204 || status === 205 ? null : new Uint8Array(data), { status, headers: responseHeaders });
          Object.defineProperty(result, "url", { value: url.href });
          resolve(result);
        } catch (error) { response.destroy(); reject(error); }
      });
      request.on("error", reject);
      request.on("upgrade", (_response, socket) => { socket.destroy(); reject(new Error("unexpected upgrade")); });
      request.end(body);
    });
  } catch { throw new Error("MCP OAuth endpoint request failed within its network limits"); }
}
