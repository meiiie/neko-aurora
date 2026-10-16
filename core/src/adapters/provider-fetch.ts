/** Model requests stay on their explicitly selected endpoint, including its path.
 * Fetch's built-in redirect handling is unsafe for provider-specific credential headers. */
import { admitProfileBridgeFetch } from "../shared/profile-bridge-guard.ts";
export class ProviderRedirectError extends Error {
  readonly name = "ProviderRedirectError";
  constructor(status: number) {
    // Never include Location, response body, request URL or header values in this error.
    super(`Provider endpoint redirected (HTTP ${status}); automatic redirects are disabled.`);
  }
}

export async function providerFetch(url: string, init: RequestInit): Promise<Response> {
  admitProfileBridgeFetch(url, init);
  const response = await fetch(url, { ...init, redirect: "manual" });
  if (response.status >= 300 && response.status < 400) {
    await response.body?.cancel().catch(() => {});
    throw new ProviderRedirectError(response.status);
  }
  return response;
}
