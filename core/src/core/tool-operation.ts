/** Runtime-owned invocation identity; model arguments and JSON-shaped copies are never authority. */
import { randomBytes } from "node:crypto";

const operations = new WeakSet<ToolOperationContext>();

export interface ToolOperationContext {
  readonly version: 1;
  readonly operationId: string;
}

/** Mint once when the Agent journals a new call, before its pre-effect checkpoint. */
export function createToolOperation(): ToolOperationContext {
  const operation = Object.freeze({ version: 1 as const, operationId: randomBytes(16).toString("hex") });
  operations.add(operation);
  return operation;
}

export function assertToolOperation(operation: ToolOperationContext | undefined): asserts operation is ToolOperationContext {
  if (!operation || !operations.has(operation)) {
    throw new Error("Host tool requires a runtime-owned checkpointed operation");
  }
}
