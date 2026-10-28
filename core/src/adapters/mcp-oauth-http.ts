import { abortable, throwIfAborted } from "../shared/abort.ts";

export async function boundedOAuthResponse(response: Response, signal: AbortSignal, maxBytes: number): Promise<Response> {
  const reader = response.body?.getReader();
  let finished = false;
  try {
    throwIfAborted(signal);
    if (!reader) return response;
    const length = response.headers.get("content-length");
    if (length && /^\d+$/.test(length) && Number(length) > maxBytes) throw new Error("oversized response");
    const bytes = new Uint8Array(maxBytes);
    let size = 0;
    while (true) {
      const { done, value } = await abortable(reader.read(), signal);
      throwIfAborted(signal);
      if (done) { finished = true; break; }
      if (value.byteLength > maxBytes - size) throw new Error("oversized response");
      bytes.set(value, size);
      size += value.byteLength;
    }
    const bounded = new Response(bytes.subarray(0, size), { status: response.status, statusText: response.statusText, headers: response.headers });
    Object.defineProperties(bounded, { url: { value: response.url }, redirected: { value: response.redirected }, type: { value: response.type } });
    return bounded;
  } catch {
    throw new Error("MCP OAuth response could not be read within its limits");
  } finally {
    if (!finished) void reader?.cancel().catch(() => {});
    reader?.releaseLock();
  }
}
