import { defineRailway, project, service, volume } from "railway/iac";
// This repository manages only its own resources in the environment. Other
// repositories export their own partial name.
// See https://docs.railway.com/infrastructure-as-code#multi-repo-projects
export const partial = "neko-aurora";
export default defineRailway(() => {
  // the existing state volume (accounts, ledgers, bot homes) must stay attached
  const data = volume("neko-aurora-volume", { region: "sfo", sizeMB: 500 });
  const nekoAurora = service("neko-aurora", {
    volumeMounts: { "/data": data },
    restartPolicyType: "ON_FAILURE",
    restartPolicyMaxRetries: 5,
    build: { buildEnvironment: "V3", builder: "DOCKERFILE", dockerfilePath: "Dockerfile", watchPatterns: ["app/native/**", "core/**", "Dockerfile"] },
    start: "neko-aurora -state /data",
    healthcheck: "/healthz",
    healthcheckTimeout: 60,
    replicas: { "sfo": 1 },
  });

  return project("neko-aurora", {
    variables: { managed: false },
    resources: [nekoAurora, data],
  });
});
