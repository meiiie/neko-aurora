import { expect, test } from "bun:test";
import { AnthropicProvider } from "../src/adapters/anthropic.ts";
import { NekoConfig } from "../src/adapters/config.ts";
import { listModels, OpenAICompatProvider } from "../src/adapters/providers.ts";
import { ResponsesProvider } from "../src/adapters/responses-provider.ts";
import { providerFetch, ProviderRedirectError } from "../src/adapters/provider-fetch.ts";

const SYNTHETIC_KEY = "synthetic-provider-key-not-a-real-credential";
const CUSTOM_SECRET = "synthetic-custom-header-not-a-real-credential";
const kinds = ["anthropic", "openai_compat", "responses", "anthropic-models", "compat-models"] as const;

for (const status of [301, 302, 303, 307, 308]) {
  for (const kind of kinds) test(`${kind} rejects HTTP ${status} before any request to a second origin`, async () => {
    let sourceRequests = 0, destinationRequests = 0;
    const destination = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch() {
      destinationRequests++;
      return Response.json({ content: [{ type: "text", text: "must not be reached" }], data: [{ id: "redirected-model" }] });
    } });
    const location = `${destination.url}credential-sink?key=${SYNTHETIC_KEY}`;
    const source = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
      sourceRequests++;
      expect(request.headers.get("authorization")).toBe(`Bearer ${SYNTHETIC_KEY}`);
      if (kind.startsWith("anthropic")) expect(request.headers.get("x-api-key")).toBe(SYNTHETIC_KEY);
      if (!kind.endsWith("models")) {
        expect(request.headers.get("x-provider-secret")).toBe(CUSTOM_SECRET);
        const body = await request.json();
        expect(body.model).toBe("glm-5.3");
      }
      return new Response(`upstream reflected ${SYNTHETIC_KEY} and ${CUSTOM_SECRET}`, { status, headers: { Location: location } });
    } });
    const cfg = new NekoConfig({ provider: kind.startsWith("anthropic") ? "anthropic" : kind === "responses" ? "responses" : "openai_compat",
      base_url: source.url.toString().replace(/\/$/, ""), model: "glm-5.3", max_retries: 5,
      offline_retry_seconds: 30, retry_base_delay_seconds: 0.001 }, null, {}, "");
    // This fixture never consults process credentials or persisted auth stores.
    Object.defineProperty(cfg, "apiKey", { get: () => SYNTHETIC_KEY });
    const headers = () => ({ "x-provider-secret": CUSTOM_SECRET, "x-api-key": SYNTHETIC_KEY });
    try {
      const invoke = () => kind.endsWith("models") ? listModels(cfg)
        : (kind === "anthropic" ? new AnthropicProvider(cfg, () => SYNTHETIC_KEY, headers)
          : kind === "responses" ? new ResponsesProvider(cfg, () => SYNTHETIC_KEY, headers)
            : new OpenAICompatProvider(cfg, () => SYNTHETIC_KEY, headers)).complete([{ role: "user", content: "Synthetic redirect probe" }]);
      let failure: unknown;
      try { await invoke(); } catch (error) { failure = error; }
      expect(failure).toBeInstanceOf(ProviderRedirectError);
      expect(String(failure)).toContain(`HTTP ${status}`);
      expect(String(failure)).not.toContain(SYNTHETIC_KEY);
      expect(String(failure)).not.toContain(CUSTOM_SECRET);
      expect(String(failure)).not.toContain(location);
      expect(sourceRequests).toBe(1); // neither offline retry nor a configured-model fallback
      expect(destinationRequests).toBe(0); // no credential header OR prompt body reaches another origin
    } finally { source.stop(true); destination.stop(true); }
  });
}

for (const status of [301, 302, 303, 307, 308]) test(`same-origin HTTP ${status} cannot change the approved model endpoint path`, async () => {
  let initialRequests = 0, redirectedRequests = 0;
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch(request) {
    if (new URL(request.url).pathname === "/approved") {
      initialRequests++;
      return new Response("secret-looking redirect body", { status, headers: { Location: "/different-path" } });
    }
    redirectedRequests++;
    return new Response("unexpected");
  } });
  try {
    await expect(providerFetch(`${server.url}approved`, { headers: { "x-api-key": SYNTHETIC_KEY, authorization: `Bearer ${SYNTHETIC_KEY}` }, redirect: "follow" }))
      .rejects.toBeInstanceOf(ProviderRedirectError);
    expect(initialRequests).toBe(1); expect(redirectedRequests).toBe(0);
  } finally { server.stop(true); }
});

test("non-redirect provider response remains readable at the exact configured endpoint", async () => {
  let requests = 0;
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch(request) {
    requests++;
    expect(new URL(request.url).pathname).toBe("/approved");
    return Response.json({ data: "exact response" });
  } });
  try {
    const response = await providerFetch(`${server.url}approved`, { headers: { "x-api-key": SYNTHETIC_KEY } });
    expect(await response.json()).toEqual({ data: "exact response" });
    expect(requests).toBe(1);
  } finally { server.stop(true); }
});

for (const route of ["paas", "coding", "coding/paas"]) test(`Z.AI Anthropic model lookup rejects ${route} protocol mismatch before resolving a credential`, async () => {
  const cfg = new NekoConfig({ provider: "anthropic", base_url: `https://api.z.ai/api/${route}/v4`, model: "glm-5.3" }, null, {}, "");
  let credentialReads = 0;
  Object.defineProperty(cfg, "apiKey", { get: () => { credentialReads++; throw new Error("must not read a key"); } });
  await expect(listModels(cfg)).rejects.toThrow("endpoint/protocol mismatch");
  await expect(new AnthropicProvider(cfg).complete([{ role: "user", content: "not sent" }])).rejects.toThrow("endpoint/protocol mismatch");
  expect(credentialReads).toBe(0);
});

for (const endpoint of ["not-a-url", `https://user:${SYNTHETIC_KEY}@example.invalid`, `https://example.invalid?key=${SYNTHETIC_KEY}`]) {
  test("invalid Anthropic endpoint is rejected without exposing endpoint credentials or reading a key", async () => {
    const cfg = new NekoConfig({ provider: "anthropic", base_url: endpoint, model: "glm-5.3" }, null, {}, "");
    let reads = 0;
    const provider = new AnthropicProvider(cfg, () => { reads++; return SYNTHETIC_KEY; });
    let failure: unknown;
    try { await provider.complete([{ role: "user", content: "not sent" }]); } catch (error) { failure = error; }
    expect(String(failure)).toContain("Invalid Anthropic endpoint URL");
    expect(String(failure)).not.toContain(endpoint);
    expect(String(failure)).not.toContain(SYNTHETIC_KEY);
    expect(reads).toBe(0);
  });
}
