/** Provider and web credentials must not be ambient capabilities of model-driven child processes. */
export const BASE_CHILD_SECRET_ENVS = [
  "NEKO_API_KEY",
  "NEKO_CREDENTIAL_HOME",
  "NEKO_PROFILE_REVISION",
  "NEKO_PROFILE_REVISION_KEY",
  "NEKO_PROFILE_LIMITS",
  "NEKO_PROFILE_ORIGIN",
  "NEKO_PROFILE_CAPSULE_PATH",
  "NEKO_PROFILE_SNAPSHOT_ID",
  "NEKO_PROFILE_AUTH_POLICY",
  "NEKO_PROFILE_CREDENTIAL_SOURCE",
  "NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS",
  "NEKO_PROFILE_DEADLINE_UNIX_MS",
  "OPENAI_API_KEY",
  "NVIDIA_API_KEY",
  "TAVILY_API_KEY",
  "NEKO_TAVILY_API_KEY",
  "JINA_API_KEY",
] as const;

/** Return a copy suitable for untrusted/agent-driven children, removing names case-insensitively. */
export function scrubChildEnv(
  source: Record<string, string | undefined>,
  additional: Iterable<string> = [],
): Record<string, string> {
  const denied = new Set(
    [...BASE_CHILD_SECRET_ENVS, ...additional]
      .map((name) => String(name).trim().toUpperCase())
      .filter(Boolean),
  );
  return Object.fromEntries(
    Object.entries(source).filter(
      (entry): entry is [string, string] => entry[1] !== undefined && !denied.has(entry[0].toUpperCase()),
    ),
  );
}
