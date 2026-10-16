import { writeNewStore } from "../../src/adapters/task-session.ts";

console.log("ready");
for await (const chunk of process.stdin) {
  if (chunk.toString().trim() !== "publish") process.exit(1);
  try {
    writeNewStore(process.argv[2]!, process.argv[3]!, "f".repeat(32));
    console.log(JSON.stringify({ published: true }));
  } catch (error) {
    // SAFETY: a native collision exposes only its errno, never any stored bytes.
    const code = (error as NodeJS.ErrnoException).code;
    console.log(JSON.stringify({ published: false, code }));
  }
  break;
}
