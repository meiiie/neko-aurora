import {assertContextSourceObservations, contextManifestDigest, contextSourceMessages, decodeContextAdmission, type ContextManifest} from "../../core/context/admission.ts";
import type {Agent} from "../../core/agent.ts";
import type {ToolRegistry} from "../../core/tool-runtime.ts";
/** Shared host-side structured compaction for CLI, TUI and embedding runtimes. */
import {createHash} from "node:crypto";
import type {AgentOptions} from "../../core/agent-constants.ts";
import {estimateRequestTokens} from "../../core/agent-constants.ts";
import {ContextProjection} from "../../core/context/projection.ts";
import {captureSourceGroup, parseGroup, renderStructuredView} from "../../core/context/structured-sources.ts";
import {decodeStructuredContextState, structuredCapsuleDigest, type StructuredContextState} from "../../core/context/state.ts";
import {assertTaskScope, type TaskScope} from "../../core/task-scope.ts";
import {isJsonObject, isText, type JsonObject} from "../../shared/wire.ts";
import {PagedSources} from "./paged-sources.ts";
import type {CredentialSecrets} from "../../shared/credential-output.ts";

interface ContextCompactorOptions {
  home: string;
  admission?: ContextManifest;
  sessionId?: string;
  policyId?: string;
  scope: TaskScope;
  messages: readonly unknown[];
  state?: StructuredContextState;
  inputBudget: () => number;
  countTokens?: (messages: readonly JsonObject[]) => number;
  credential: () => CredentialSecrets | undefined;
  publish: (expectedWorkingDigest: string, messages: JsonObject[], state: StructuredContextState) => {committed: true};
}

function guardCredential(messages: readonly JsonObject[], credential: CredentialSecrets | undefined): void {
  const secrets = (isText(credential) ? [credential] : credential ?? []).filter(secret => secret.length >= 8);
  if (!secrets.length) return;
  const pending: unknown[] = [...messages];
  while (pending.length) {
    const value = pending.pop();
    if (isText(value)) { if (secrets.some(secret => value.includes(secret))) throw new Error("Structured context contains a configured credential; archive blocked"); }
    else if (Array.isArray(value)) { for (const item of value) pending.push(item); }
    else if (isJsonObject(value)) for (const [key, item] of Object.entries(value)) pending.push(key, item);
  }
}

function groups(messages: readonly JsonObject[]): JsonObject[][] {
  const result: JsonObject[][] = [];
  for (const message of messages) {
    if (message.role === "system" || message._neko_context_capsule === true) continue;
    if (message.role === "user") result.push([]);
    if (!result.length) throw new Error("Context snapshot does not start at a user boundary");
    result.at(-1)!.push(structuredClone(message));
  }
  return result;
}

export class StructuredContextCompactor {
  private state: StructuredContextState;
  private journal: PagedSources;

  constructor(private readonly options: ContextCompactorOptions) {
    assertTaskScope(options.scope);
    this.journal = new PagedSources(options.home, options.scope, options.state?.journal);
    this.state = options.state
      ? decodeStructuredContextState(options.state, options.messages, options.scope.id, options.scope.canonicalRoot, options.scope.executionAuthorityId, options.sessionId)
      : {version: 1, journal: this.journal.reference(), capsule: null};
    if (options.admission) {
      if (this.state.admission) throw new Error("context source cannot be admitted twice");
      const messages = contextSourceMessages(options.admission);
      guardCredential(messages, options.credential());
      const source = captureSourceGroup(options.scope, this.journal.nextSequence(), messages, [], "runtime");
      this.journal.append(source);
      const admission = decodeContextAdmission({manifest: options.admission, manifestSHA256: contextManifestDigest(options.admission), sourceIds: [source.id],
        binding: {sessionId: options.sessionId, taskId: options.scope.id, root: options.scope.canonicalRoot,
          authorityId: options.scope.executionAuthorityId, policyId: options.policyId}}, options.scope.id, options.scope.canonicalRoot,
        options.scope.executionAuthorityId, options.sessionId);
      this.state = {...this.state, admission, journal: this.journal.reference()};
    }
    if (this.state.admission) {
      if (this.state.admission.binding.policyId !== options.policyId) throw new Error("context execution policy changed");
      const group = parseGroup(options.scope, this.journal.read(this.state.admission.sourceIds[0]));
      if (group.origin !== "runtime" || group.facts.length || JSON.stringify(group.messages) !== JSON.stringify(contextSourceMessages(this.state.admission.manifest))) {
        throw new Error("context admission does not match its native source journal");
      }
      guardCredential(group.messages, options.credential());
    }
  }

  snapshot(messages: readonly unknown[]): StructuredContextState {
    if (this.state.admission) assertContextSourceObservations(messages, this.state.admission, this.options.scope.activationEpoch);
    // An explicit clear may remove the working capsule, but never deletes immutable source history.
    const hasCapsule = messages.some(message => isJsonObject(message) && message._neko_context_capsule === true);
    const state = hasCapsule ? this.state : {...this.state, capsule: null};
    return decodeStructuredContextState(state, messages, this.options.scope.id, this.options.scope.canonicalRoot, this.options.scope.executionAuthorityId, this.options.sessionId);
  }

  install(messages: JsonObject[]): void {
    const admission = this.state.admission;
    if (!admission) return;
    const expected = contextSourceMessages(admission.manifest);
    const existing = messages.filter(message => message._neko_context_admission !== undefined).map(message => {
      const clean = {...message}; delete clean._neko_acp_message_id; return clean;
    });
    if (existing.length && JSON.stringify(existing) !== JSON.stringify(expected)) throw new Error("context working source metadata changed");
    if (!existing.length) {
      const offset = messages.findIndex(message => message.role !== "system");
      messages.splice(offset < 0 ? messages.length : offset, 0, ...expected);
    }
  }

  async lookup(id: string): Promise<string | undefined> {
    assertTaskScope(this.options.scope);
    if (!/^[a-f0-9]{64}$/.test(id)) return undefined;
    const journal = this.journal;
    try { await journal.assertContainsAsync([id]); }
    catch (error) {
      if (error instanceof Error && error.message.includes("outside the selected journal branch")) return undefined;
      throw error;
    }
    assertTaskScope(this.options.scope);
    const source = journal.read(id);
    // Parse only the validated native group for the credential guard; no model instruction is executed.
    const parsed: unknown = JSON.parse(source.text);
    if (!isJsonObject(parsed) || !Array.isArray(parsed.messages) || !parsed.messages.every(isJsonObject)) throw new Error("Invalid context source");
    guardCredential(parsed.messages, this.options.credential());
    return JSON.stringify({version: parsed.version, sequence: parsed.sequence, origin: parsed.origin,
      messages: parsed.messages, unknownOutcomes: parsed.unknownOutcomes, facts: parsed.facts});
  }

  readonly prepare: NonNullable<AgentOptions["prepareCompaction"]> = async ({before, candidate}, signal) => {
    assertTaskScope(this.options.scope); signal?.throwIfAborted();
    this.snapshot(before);
    if (!before.every(isJsonObject) || !candidate.every(isJsonObject)) throw new Error("Invalid native context snapshot");
    guardCredential(before, this.options.credential());
    const originalDigest = createHash("sha256").update(JSON.stringify(before)).digest("hex");
    const prefix = candidate.filter(message => message.role === "system");
    if (this.state.admission) prefix.push(...contextSourceMessages(this.state.admission.manifest));
    const capsule = candidate.find(message => message.role === "user" && message._neko_internal === true);
    if (!capsule) throw new Error("Compaction candidate has no host capsule");
    const tail = candidate.slice(candidate.indexOf(capsule) + 1).filter(message => message._neko_context_admission === undefined);
    const conversation = before.filter(message => message.role !== "system");
    const head = conversation.slice(0, conversation.length - tail.length);
    const candidateJournal = new PagedSources(this.options.home, this.options.scope, this.state.journal);
    const headIds: string[] = [], tailIds: string[] = [];
    for (const [items, ids] of [[groups(head), headIds], [groups(tail), tailIds]] as const) {
      for (const messages of items) {
        signal?.throwIfAborted();
        const unknown = messages.filter(message => message.role === "tool" && isText(message.content)
          && message.content.startsWith("[interrupted while this tool call was in flight; outcome unknown."))
          .map(message => String(message.tool_call_id));
        const source = captureSourceGroup(this.options.scope, candidateJournal.nextSequence(), messages, unknown,
          messages[0]._neko_internal === true ? "runtime" : "user");
        await candidateJournal.appendAsync(source, signal);
        ids.push(source.id);
      }
    }
    const noteIds = (headIds.length ? headIds : this.state.capsule?.sources ?? []).slice(-8);
    if (!noteIds.length) throw new Error("Compaction has no scoped historical source");
    await candidateJournal.assertContainsAsync([...noteIds, ...tailIds], signal);
    const view = new ContextProjection(this.options.scope, prefix.map(message => JSON.stringify(message)), 8 * 1024 * 1024, () => {});
    for (const id of new Set([...noteIds, ...tailIds])) view.append(this.options.scope, candidateJournal.read(id));
    view.apply(this.options.scope, view.snapshot(this.options.scope).revision,
      [{kind: "note", text: String(capsule.content), sources: noteIds}, ...tailIds.map(id => ({kind: "source", id}))]);
    const replacement = renderStructuredView(this.options.scope, view, prefix, this.options.inputBudget(), this.options.countTokens ?? (messages => estimateRequestTokens([...messages])),
      undefined, runtime => candidateJournal.facts(runtime));
    const note = replacement[prefix.length];
    note._neko_internal = true; note._neko_context_capsule = true; note._neko_context_sources = noteIds;
    for (const key of ["_neko_compaction_first_user", "_neko_compaction_source_ids", "_neko_compaction_source_digest"]) {
      if (capsule[key] !== undefined) note[key] = structuredClone(capsule[key]);
    }
    guardCredential(replacement, this.options.credential());
    const state: StructuredContextState = {version: 1, ...(this.state.admission ? {admission: this.state.admission} : undefined), journal: candidateJournal.reference(),
      capsule: {messageDigest: structuredCapsuleDigest(note), sources: noteIds}};
    signal?.throwIfAborted();
    return {messages: replacement, commit: () => {
      assertTaskScope(this.options.scope);
      const receipt = this.options.publish(originalDigest, replacement, state);
      this.state = state; this.journal = candidateJournal;
      return receipt;
    }};
  };
}


/** One shared binding path; client UIs do not own a second memory/compaction algorithm. */
export function bindStructuredContext(options: ContextCompactorOptions & {agent: Agent; registry: ToolRegistry}) {
  const context = new StructuredContextCompactor(options);
  context.install(options.agent.messages);
  options.agent.bindCompactionPreparation(context.prepare);
  options.registry.bindContextSourceLookup(options.scope, id => context.lookup(id));
  return context;
}
