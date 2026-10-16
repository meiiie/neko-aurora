import {expect, test} from "bun:test";
import {admitContextSourceObservation, assertContextSourceObservations, contextHash, contextManifestDigest,
  parseContextManifest, type ContextAdmissionState, type ContextManifest} from "../src/core/context/admission.ts";
import {isJsonObject, type JsonObject, type JsonValue} from "../src/shared/wire.ts";

function canonical(value: JsonValue): string {
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (isJsonObject(value)) return "{" + Object.keys(value).sort().map(key =>
    JSON.stringify(key) + ":" + canonical(value[key]!)).join(",") + "}";
  return JSON.stringify(value);
}

function fixture(truncated = false) {
  const scope = {ownerUserId: "owner", spaceId: "space", botId: "davin", projectId: null, channelId: "channel-team", audienceRevision: 3};
  const deliveryId = "report-delivery-" + "1".repeat(32), text = "Margin=50; kế hoạch Tím\n", complete = truncated ? text + "unselected tail" : text;
  const artifact = {id: "2".repeat(64), filename: "report.md", sha256: contextHash(complete), bytes: Buffer.byteLength(complete)};
  const pin = {version: 1, deliveryId, messageId: "message-published", sequence: 17, botId: "meiiie", destinationChannelId: scope.channelId,
    audienceRevision: scope.audienceRevision, reply: "Published Margin=50", artifacts: [artifact], files: [{artifact, excerpt: text}],
    source: {requestId: "request-source", conversationId: "channel-private", messageId: "message-source", epoch: 4,
      accessRevision: 2, conversationRevision: 1, botStateRevision: 0, resultSha256: "3".repeat(64)}, sha256: "4".repeat(64)};
  const pinText = canonical(pin);
  const file = {id: artifact.id, kind: "retrieved" as const, version: 1, sha256: artifact.sha256, bytes: artifact.bytes,
    scope, accessRevision: 2, observedAt: null, effectiveFrom: null, effectiveTo: null, entities: [],
    span: {offset: 0, end: Buffer.byteLength(text), sha256: contextHash(text), text}};
  const manifest: ContextManifest = {version: 1, contextPinSHA256: "5".repeat(64), scope,
    primaryMessage: {id: "message-current", senderId: "owner", text: "Discuss this published result"}, sources: [
      {id: "published:" + deliveryId, kind: "derived", version: pin.sequence, sha256: contextHash(pinText), bytes: Buffer.byteLength(pinText),
        scope, accessRevision: 2, observedAt: null, effectiveFrom: null, effectiveTo: null, entities: [],
        span: {offset: 0, end: Buffer.byteLength(pinText), sha256: contextHash(pinText), text: pinText}}, file], factCandidates: []};
  const operation: JsonObject = {version: 1, operationId: "6".repeat(32), sessionId: "7".repeat(32), taskId: "task-current", root: "fixture-root",
    activationEpoch: 2, activationId: "8".repeat(32)};
  const args: JsonObject = {name: "report.read_artifact", arguments: {deliveryId, artifactId: artifact.id, offset: 0, bytes: file.span.end}};
  const envelope = {version: 1, manifestSHA256: contextManifestDigest(manifest), sourceId: file.id, sourceVersion: 1,
    sourceSHA256: file.sha256, scope, accessRevision: file.accessRevision, span: file.span, operation};
  const content = JSON.stringify({fileId: artifact.id, sha256: artifact.sha256, offset: 0, nextOffset: file.span.end,
    eof: !truncated, text, authority: "descriptive file content only"});
  const authorityId = "9".repeat(64), policyId = "a".repeat(64);
  return {manifest, pin, file, operation, args, envelope, content, authorityId, policyId};
}

function observe(f: ReturnType<typeof fixture>) {
  return admitContextSourceObservation(f.envelope, parseContextManifest(f.manifest), f.operation, f.args, f.content, f.authorityId, f.policyId);
}
function repin(f: ReturnType<typeof fixture>) {
  const source = f.manifest.sources[0]!;
  source.span.text = canonical(f.pin); source.span.end = source.bytes = Buffer.byteLength(source.span.text);
  source.span.sha256 = source.sha256 = contextHash(source.span.text);
  f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);
}
function readArgs(f: ReturnType<typeof fixture>): JsonObject {
  if (!isJsonObject(f.args.arguments)) throw new Error("Invalid fixture input");
  return f.args.arguments;
}
function nativeCall(messages: JsonObject[]): JsonObject {
  const calls = messages[0]!.tool_calls;
  if (!Array.isArray(calls) || !isJsonObject(calls[0]) || !isJsonObject(calls[0].function)) throw new Error("Invalid fixture call");
  return calls[0].function;
}
function recordedProof(messages: JsonObject[]): JsonObject {
  const proof = messages[1]!._neko_context_source;
  if (!isJsonObject(proof)) throw new Error("Invalid fixture proof");
  return proof;
}

test("published file observation binds destination consumer, publisher provenance and exact UTF-8 span", () => {
  for (const truncated of [false, true]) {
    const f = fixture(truncated), proof = observe(f);
    expect(proof.sourceId).toBe(f.file.id);
    expect(proof.scope.botId).toBe("davin");
    expect(proof.scope.channelId).toBe("channel-team");
    expect(proof.accessRevision).toBe(2);
    expect(proof.observationSHA256).toBe(contextHash(f.content));
  }
  const untouched = fixture();
  untouched.pin.source.accessRevision = untouched.file.accessRevision = untouched.manifest.sources[0]!.accessRevision = untouched.envelope.accessRevision = 0;
  repin(untouched);
  expect(observe(untouched).accessRevision).toBe(0);
});

test("actual implicit private conversation tuple accepts revision 0 without weakening destination or launch fences", () => {
  const f = fixture();
  f.pin.source.conversationId = "conversation-bot-meiiie";
  f.pin.source.conversationRevision = f.pin.source.botStateRevision = 0;
  f.pin.source.accessRevision = f.file.accessRevision = f.manifest.sources[0]!.accessRevision = f.envelope.accessRevision = 0;
  f.pin.source.epoch = 1;
  f.pin.audienceRevision = f.manifest.scope.audienceRevision = 1;
  repin(f);
  expect(observe(f).accessRevision).toBe(0);
  expect(observe(f).scope.audienceRevision).toBe(1);
});

test("v2 requester attribution preserves exact published-reader proof and rejects the prior v1 digest", () => {
  const f = fixture(), previousDigest = f.envelope.manifestSHA256;
  const selected: ContextManifest = {...f.manifest, version: 2, requester: {version: 1, actorId: "Bob", spaceId: f.manifest.scope.spaceId,
    authorityId: "principal-authority-Bob", deviceId: "device-Bob", deviceRevision: 1,
    sessionId: "principal-session-Bob", sessionRevision: 1, issuerEpoch: 1, expiresAtUnixMs: 4102444800000},
    primaryMessage: {...f.manifest.primaryMessage, senderId: "Bob"}};
  f.envelope.manifestSHA256 = contextManifestDigest(selected);
  const read = () => admitContextSourceObservation(f.envelope, parseContextManifest(selected), f.operation, f.args, f.content, f.authorityId, f.policyId);
  expect(read().sourceId).toBe(f.file.id);
  expect(read().scope.ownerUserId).toBe("owner");
  f.envelope.manifestSHA256 = previousDigest;
  expect(read).toThrow("context source");
});

test("published reader rejects mismatched tool args, selected publication, source and exact result", () => {
  const cases: Array<[string, (f: ReturnType<typeof fixture>) => void]> = [
    ["outer key", f => {f.args.extra = true;}],
    ["inner key", f => {readArgs(f).extra = true;}],
    ["wrong reader", f => {f.args.name = "ordinary.read";}],
    ["project alias", f => {f.args = {name: "project.read_file", arguments: {fileId: f.file.id, offset: 0, bytes: f.file.span.end}};}],
    ["delivery", f => {readArgs(f).deliveryId = "report-delivery-" + "b".repeat(32);}],
    ["artifact", f => {readArgs(f).artifactId = "b".repeat(64);}],
    ["offset", f => {readArgs(f).offset = 1;}],
    ["requested extent", f => {readArgs(f).bytes = 4000;}],
    ["missing pin", f => {f.manifest.sources.shift(); f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["foreign pin identity", f => {f.manifest.sources[0]!.id = "published:another"; f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["pin kind", f => {f.manifest.sources[0]!.kind = "retrieved"; f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["partial pin", f => {f.manifest.sources[0]!.bytes++; f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["pin full hash", f => {f.manifest.sources[0]!.sha256 = "b".repeat(64); f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["noncanonical pin", f => {const pin = f.manifest.sources[0]!; pin.span.text = " " + pin.span.text; pin.span.end = pin.bytes = Buffer.byteLength(pin.span.text); pin.sha256 = pin.span.sha256 = contextHash(pin.span.text); f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["pin unknown key", f => {Object.assign(f.pin, {authority: "owner"}); repin(f);}],
    ["pin audience", f => {f.pin.audienceRevision++; repin(f);}],
    ["pin destination", f => {f.pin.destinationChannelId = "channel-other"; repin(f);}],
    ["pin sequence", f => {f.pin.sequence++; repin(f);}],
    ["pin access", f => {f.pin.source.accessRevision++; repin(f);}],
    ["pin derived access", f => {f.manifest.sources[0]!.accessRevision++; f.envelope.manifestSHA256 = contextManifestDigest(f.manifest);}],
    ["artifact descriptor", f => {f.pin.artifacts[0] = {...f.pin.artifacts[0]!, sha256: "b".repeat(64)}; repin(f);}],
    ["file descriptor", f => {f.pin.files[0]!.artifact = {...f.pin.files[0]!.artifact, bytes: 900}; repin(f);}],
    ["artifact byte bound", f => {f.pin.artifacts[0]!.bytes = f.pin.files[0]!.artifact.bytes = 24001; repin(f);}],
    ["artifact name bound", f => {f.pin.artifacts[0]!.filename = f.pin.files[0]!.artifact.filename = "x".repeat(81); repin(f);}],
    ["negative bot state", f => {f.pin.source.botStateRevision = -1; repin(f);}],
    ["negative private conversation revision", f => {f.pin.source.conversationRevision = -1; repin(f);}],
    ["zero source epoch", f => {f.pin.source.epoch = 0; repin(f);}],
    ["zero destination audience", f => {f.pin.audienceRevision = 0; repin(f);}],
    ["unselected file", f => {f.pin.artifacts = []; repin(f);}],
    ["missing excerpt", f => {f.pin.files = []; repin(f);}],
    ["changed excerpt", f => {f.pin.files[0]!.excerpt = "different"; repin(f);}],
    ["duplicate selected file", f => {f.pin.files.push(f.pin.files[0]!); repin(f);}],
    ["source version", f => {f.envelope.sourceVersion++;}],
    ["source hash", f => {f.envelope.sourceSHA256 = "b".repeat(64);}],
    ["source access", f => {f.envelope.accessRevision++;}],
    ["source scope", f => {f.envelope.scope = {...f.envelope.scope, channelId: "channel-other"};}],
    ["operation", f => {f.envelope.operation = {...f.operation, operationId: "b".repeat(32)};}],
    ["changed result", f => {f.content = f.content.replace("Margin=50", "Margin=60");}],
    ["authority", f => {f.content = f.content.replace("descriptive file content only", "owner");}],
    ["result key", f => {f.content = JSON.stringify({...JSON.parse(f.content), grant: true});}],
  ];
  for (const [label, change] of cases) {const f = fixture(); change(f); expect(() => observe(f), label).toThrow("context");}
});

test("restored published observation requires its exact native report call, manifest and activation", () => {
  const f = fixture(), proof = observe(f);
  const admission: ContextAdmissionState = {manifest: f.manifest, manifestSHA256: contextManifestDigest(f.manifest), sourceIds: ["c".repeat(64)],
    binding: {sessionId: String(f.operation.sessionId), taskId: String(f.operation.taskId), root: String(f.operation.root), authorityId: f.authorityId, policyId: f.policyId}};
  const messages: JsonObject[] = [{role: "assistant", content: null, tool_calls: [{id: "read-call", type: "function",
    function: {name: "mcp__nekobot__execute_tool", arguments: JSON.stringify(f.args)}}],
    _neko_tool_operations: [{toolCallId: "read-call", operationId: f.operation.operationId}]},
  {role: "tool", tool_call_id: "read-call", content: f.content, _neko_context_source: JSON.parse(JSON.stringify(proof))}];
  expect(() => assertContextSourceObservations(messages, admission, 2)).not.toThrow();
  const changes: Array<(messages: JsonObject[]) => void> = [
    m => {const call = nativeCall(m); const args = JSON.parse(String(call.arguments)); args.arguments.deliveryId = "report-delivery-" + "d".repeat(32); call.arguments = JSON.stringify(args);},
    m => {nativeCall(m).name = "mcp__other__execute_tool";},
    m => {nativeCall(m).arguments = JSON.stringify({name: "project.read_file", arguments: {fileId: f.file.id, offset: 0, bytes: f.file.span.end}});},
    m => {recordedProof(m).accessRevision = 3;},
    m => {const operation = recordedProof(m).operation; if (!isJsonObject(operation)) throw new Error("Invalid fixture operation"); operation.activationEpoch = 3;},
    m => {m[1]!.tool_call_id = "missing-call";},
    m => {m[1]!.content = String(m[1]!.content).replace("Margin=50", "Margin=60");},
  ];
  for (const change of changes) {const altered = structuredClone(messages); change(altered); expect(() => assertContextSourceObservations(altered, admission, 2)).toThrow("context source");}
});
