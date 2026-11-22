import { expect, test } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { balance, importGraph, selectTests } from "./core-tests";

const files = ["src/a.ts", "src/b.ts", "test/a.test.ts", "test/b.test.ts"];
const graph = new Map([
  ["src/b.ts", new Set(["src/a.ts"])],
  ["test/a.test.ts", new Set(["src/b.ts"])],
  ["test/b.test.ts", new Set<string>()],
]);

test("selects transitive import dependents and directly changed tests", () => {
  expect(selectTests(files, ["src/a.ts"], graph, new Set()).files).toEqual(["test/a.test.ts"]);
  expect(selectTests(files, ["test/b.test.ts"], graph, new Set()).files).toEqual(["test/b.test.ts"]);
});

test("empty changes select nothing; configuration, helpers, deletions and assets select all", () => {
  expect(selectTests(files, [], graph, new Set()).files).toEqual([]);
  for (const change of ["package.json", "bun.lock", "tsconfig.json", "bunfig.toml", "test/setup.ts", "src/deleted.ts", "src/icon.svg"]) {
    expect(selectTests(files, [change], graph, new Set()).files).toEqual(files.slice(2));
  }
});

test("unknown source reachability and changed unresolved modules fall back; opaque readers are always included", () => {
  expect(selectTests(files, ["src/b.ts"], graph, new Set(["src/b.ts"])).files).toEqual(files.slice(2));
  expect(selectTests(files, ["src/a.ts"], graph, new Set(["test/b.test.ts"])).files).toEqual(files.slice(2));
  expect(selectTests([...files, "src/fixture.ts"], ["src/fixture.ts"], graph, new Set()).files).toEqual(files.slice(2));
});

test("greedy shards are deterministic, preserve all files, default unknowns and support empty shards", () => {
  const shards = balance(["a", "b", "c", "d"], { a: 8, b: 4, c: 3 }, 2);
  expect(shards.map(shard => shard.seconds)).toEqual([8, 8]);
  expect(shards.flatMap(shard => shard.files).sort()).toEqual(["a", "b", "c", "d"]);
  expect(balance([], {}, 4).every(shard => shard.files.length === 0)).toBe(true);
  expect(balance(["new"], {}, 4)[0].seconds).toBe(1);
  expect(() => balance([], {}, 0)).toThrow();
});

test("graph resolves relative imports, type-only imports and tsconfig aliases; flags missing and computed imports", () => {
  const directory = mkdtempSync(join(tmpdir(), "ci-graph-"));
  try {
    mkdirSync(join(directory, "src"));
    mkdirSync(join(directory, "test"));
    writeFileSync(join(directory, "tsconfig.json"), JSON.stringify({ compilerOptions: { baseUrl: ".", paths: { "@/*": ["src/*"] } } }));
    writeFileSync(join(directory, "src/a.ts"), "export type A = string; export const a = 1;");
    writeFileSync(join(directory, "src/b.ts"), 'import type {A} from "./a"; export const b: A = "b";');
    writeFileSync(join(directory, "test/a.test.ts"), 'import {a} from "@/a"; import "../src/b";');
    writeFileSync(join(directory, "test/b.test.ts"), 'import "./missing"; const name = "dynamic"; import(name); readFileSync(name);');
    const result = importGraph(files, directory);
    expect([...result.graph.get("src/b.ts")!]).toEqual(["src/a.ts"]);
    expect([...result.graph.get("test/a.test.ts")!].sort()).toEqual(["src/a.ts", "src/b.ts"]);
    expect([...result.unsafe]).toEqual(["test/b.test.ts"]);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
