import { auroraCatalogError, auroraChatGptLogin, auroraChatGptModels, auroraProviderCatalog } from "../src/adapters/aurora-setup.ts";
import { hasChatGptCredentials, openBrowser } from "../src/adapters/chatgpt-auth.ts";

type SetupEvent = { state: "waiting" | "connected" | "failed"; url?: string }
  | { providers: ReturnType<typeof auroraProviderCatalog>; chatgpt: boolean };
const emit = (event: SetupEvent | Awaited<ReturnType<typeof auroraChatGptModels>> | {error:string}) => process.stdout.write(JSON.stringify(event) + "\n");
const controller = new AbortController();
process.on("SIGTERM", () => controller.abort());
process.on("SIGINT", () => controller.abort());
try {
  switch (process.argv[2]) {
    case "catalog": emit({ providers: auroraProviderCatalog(), chatgpt: hasChatGptCredentials() }); break;
    case "login": await auroraChatGptLogin(controller.signal, emit, undefined, openBrowser); break;
    case "models": emit(await auroraChatGptModels(AbortSignal.any([controller.signal, AbortSignal.timeout(25_000)]))); break;
    default: throw new Error("Unknown setup operation");
  }
} catch (error) {
  // Provider responses and callback details never cross the public setup boundary.
  if (process.argv[2] === "models") emit({error:auroraCatalogError(error instanceof Error ? error.message : "")});
  else emit({ state: "failed" });
  process.exitCode = 1;
}
