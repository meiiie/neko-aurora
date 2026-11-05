import { auroraCatalogError, auroraChatGptLogin, auroraChatGptModels, auroraProviderCatalog } from "../src/adapters/aurora-setup.ts";
import { hasChatGptCredentials, openBrowser } from "../src/adapters/chatgpt-auth.ts";
import { auroraApiModels, AURORA_ROUTES } from "../src/adapters/aurora-models.ts";
import { DEFAULTS } from "../src/adapters/config.ts";

type SetupEvent = { state: "waiting" | "connected" | "failed"; url?: string }
  | { providers: ReturnType<typeof auroraProviderCatalog>; chatgpt: boolean };
const emit = (event: SetupEvent | Awaited<ReturnType<typeof auroraChatGptModels>> | Awaited<ReturnType<typeof auroraApiModels>> | {error:string}) => process.stdout.write(JSON.stringify(event) + "\n");
const controller = new AbortController();
process.on("SIGTERM", () => controller.abort());
process.on("SIGINT", () => controller.abort());
try {
  switch (process.argv[2]) {
    case "catalog": emit({ providers: auroraProviderCatalog(), chatgpt: hasChatGptCredentials() }); break;
    case "login": await auroraChatGptLogin(controller.signal, emit, undefined, openBrowser); break;
    case "models": {
      const provider = process.argv[3] || "chatgpt";
      const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(25_000)]);
      if (!AURORA_ROUTES.some(id => id === provider)) throw new Error("Unknown catalog route");
      emit(provider === "chatgpt" ? await auroraChatGptModels(signal)
        : await auroraApiModels(provider, process.env[DEFAULTS.profiles[provider].key_env || ""] || "", signal));
      break;
    }
    default: throw new Error("Unknown setup operation");
  }
} catch (error) {
  // Provider responses and callback details never cross the public setup boundary.
  if (process.argv[2] === "models") emit({error:auroraCatalogError(error instanceof Error ? error.message : "")});
  else emit({ state: "failed" });
  process.exitCode = 1;
}
