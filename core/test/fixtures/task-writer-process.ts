import { createTaskSession, loadTaskSession, type TaskRuntimeInput } from "../../src/adapters/task-session.ts";

const home = process.argv[2]!;
const root = process.argv[3]!;
const resume = process.argv[4];
const factory = (input: TaskRuntimeInput) => ({
  messages: input.messages,
  getMessages: () => input.messages,
  getSourceEvents: () => input.sourceEvents,
  assertQuiescent() {},
  close() {},
});
const options = { home, root, authorityId: "fixture", configId: "a".repeat(64), runtimeFactory: factory,
  taskProtocol: { version: 1 as const, mode: "fixed-active-task" as const } };
try {
  const session = resume ? await loadTaskSession({ ...options, sessionId: JSON.parse(resume).sessionId,
    expectedTask: JSON.parse(resume).task }) : await createTaskSession({ ...options, label: "writer recovery fixture" });
  if (!resume) { session.active.runtime.messages.push({ role: "user", content: "durable fixture" }); session.checkpoint(); }
  console.log(JSON.stringify({ sessionId: session.id, task: session.receipt, messages: session.active.runtime.messages }));
  for await (const chunk of process.stdin) {
    if (chunk.toString().trim() === "close") { await session.close(); console.log("closed"); break; }
  }
} catch (error) {
  console.log(JSON.stringify({ error: error instanceof Error ? error.name : "unexpected" }));
  process.exit(1);
}
