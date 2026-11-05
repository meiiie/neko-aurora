import { AURORA_INPUT_LIMIT, AuroraMessages, auroraCompletion, auroraCompletionError } from "../src/adapters/aurora-completion.ts";
import { AURORA_ROUTES } from "../src/adapters/aurora-models.ts";
import { loadConfig } from "../src/adapters/config.ts";
import { getProvider } from "../src/adapters/providers.ts";

const controller = new AbortController();
process.on("SIGTERM", () => controller.abort());
process.on("SIGINT", () => controller.abort());
try {
  const profile = process.argv[2];
  if (!AURORA_ROUTES.some(id => id === profile)) throw new Error("core_configuration_invalid");
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of process.stdin) {
    const bytes = Buffer.from(chunk);
    size += bytes.length;
    if (size > AURORA_INPUT_LIMIT) throw new Error("core_configuration_invalid");
    chunks.push(bytes);
  }
  const messages = AuroraMessages.parse(JSON.parse(Buffer.concat(chunks).toString("utf8")));
  const config = loadConfig({ profile });
  await auroraCompletion(messages, getProvider(config), controller.signal, text => process.stdout.write(text));
} catch (error) {
  process.stderr.write(auroraCompletionError(error instanceof Error ? error : new Error("core_failed")) + "\n");
  process.exitCode = 1;
}
