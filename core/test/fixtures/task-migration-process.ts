import { migrateTaskSessionV1, readTaskSessionV1MigrationManifest } from "../../src/adapters/task-session-migration-v1.ts";

const phase = process.argv[3];
const report = await migrateTaskSessionV1(readTaskSessionV1MigrationManifest(process.argv[2]!), {
  async afterPhase(current) {
    if (current === phase) {
      console.log(JSON.stringify({ phase: current }));
      await new Promise<void>(() => {});
    }
  },
});
console.log(JSON.stringify(report));
