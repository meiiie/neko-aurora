import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, relative, resolve } from "node:path";

export const root = resolve(import.meta.dir, "../..");
const core = resolve(root, "core");
const timingsPath = resolve(import.meta.dir, "core-test-timings.json");
const testPattern = /\.(test|spec)\.[cm]?[jt]sx?$/;
const codePattern = /\.[cm]?[jt]sx?$/;

export function balance(files: string[], timings: Record<string, number>, count: number) {
  if (!Number.isInteger(count) || count < 1) throw new Error("shards must be a positive integer");
  const shards = Array.from({ length: count }, () => ({ files: [] as string[], seconds: 0 }));
  for (const file of [...new Set(files)].sort((a, b) => (timings[b] ?? 1) - (timings[a] ?? 1) || a.localeCompare(b))) {
    const shard = shards.reduce((a, b) => a.seconds <= b.seconds ? a : b);
    shard.files.push(file);
    shard.seconds += timings[file] ?? 1;
  }
  return shards;
}

export function selectTests(files: string[], changed: string[], dependencies: Map<string, Set<string>>, unsafe: Set<string>) {
  const tests = files.filter(file => testPattern.test(file));
  const full = (reason: string) => ({ files: tests, reason });
  if (changed.some(file => !codePattern.test(file) || !files.includes(file)
      || (!testPattern.test(file) && !file.startsWith("src/")))) return full("configuration, helper, asset or deleted file changed");
  if (!changed.length) return { files: [], reason: "no Core changes" };
  if (changed.some(file => unsafe.has(file))) return full("changed file has unresolved imports or computed module loading");
  const affected = new Set([...changed, ...unsafe]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const [file, imports] of dependencies) {
      if (!affected.has(file) && [...imports].some(dependency => affected.has(dependency))) {
        affected.add(file);
        grew = true;
      }
    }
  }
  const selected = tests.filter(file => affected.has(file));
  if (changed.some(file => !testPattern.test(file) && !tests.some(test => reaches(test, file, dependencies)))) {
    return full("changed source has no statically reachable test (possible runtime fixture)");
  }
  return { files: selected, reason: "transitive import dependents" };
}

function reaches(start: string, target: string, graph: Map<string, Set<string>>, seen = new Set<string>()): boolean {
  if (start === target) return true;
  if (seen.has(start)) return false;
  seen.add(start);
  return [...(graph.get(start) ?? [])].some(next => reaches(next, target, graph, seen));
}

export function importGraph(files: string[], directory = core) {
  const config = JSON.parse(readFileSync(resolve(directory, "tsconfig.json"), "utf8"));
  if (config.extends) throw new Error("extended tsconfig requires full selection");
  const graph = new Map<string, Set<string>>();
  const unsafe = new Set<string>();
  const known = new Set(files);
  for (const file of files.filter(file => codePattern.test(file))) {
    const absolute = resolve(directory, file);
    const source = readFileSync(absolute, "utf8");
    const imports = new Set<string>();
    const add = (specifier: string) => {
      const local = specifier.startsWith(".") || specifier.startsWith("/")
        || Object.keys(config.compilerOptions?.paths ?? {}).some(key => specifier.startsWith(key.split("*")[0]));
      if (!local) return;
      try {
        const dependency = relative(directory, Bun.resolveSync(specifier, dirname(absolute))).replaceAll("\\", "/");
        if (known.has(dependency)) imports.add(dependency); else unsafe.add(file);
      } catch {
        unsafe.add(file);
      }
    };
    try {
      const scanner = new Bun.Transpiler({ loader: /x$/.test(file) ? "tsx" : "ts" });
      for (const item of scanner.scanImports(source)) add(item.path);
      for (const match of source.matchAll(/\bfrom\s*["']([^"']+)["']/g)) add(match[1]);
      if (/\b(?:import|require)\s*\(\s*[^\s"'`]/.test(source) || /\b(?:import|require)\s*\(\s*`[^`]*\$\{/.test(source)) unsafe.add(file);
      if (/\b(?:readFileSync|readFile|readdirSync|readdir|globSync|spawnSync|spawn|execFile|execSync|Worker)\s*\(|\bBun\s*\.\s*file\s*\(/.test(source)) unsafe.add(file);
    } catch {
      unsafe.add(file);
    }
    graph.set(file, imports);
  }
  return { graph, unsafe };
}

function git(...args: string[]) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).split("\0").filter(Boolean);
}

if (import.meta.main) {
  const args = process.argv.slice(2);
  const value = (flag: string, fallback: string) => args.includes(flag) ? args[args.indexOf(flag) + 1] : fallback;
  if (args.includes("--refresh")) {
    const input = readFileSync(value("--refresh", ""), "utf8");
    const timings: Record<string, number> = {};
    if (input.startsWith("file\t")) {
      for (const row of input.trim().split("\n").slice(1)) {
        const [file, seconds] = row.split("\t");
        if (!Number.isFinite(Number(seconds))) throw new Error(`invalid timing: ${file}`);
        timings[file] = Number((Number(seconds) + 0.1).toFixed(6));
      }
    } else {
      let file = "";
      for (const line of input.replace(/\x1b\[[0-9;]*m/g, "").split("\n")) {
        if (/^\S+\.(test|spec)\.[cm]?[jt]sx?:$/.test(line)) { file = line.slice(0, -1); timings[file] ??= 0.1; }
        const match = line.match(/^\((?:pass|fail)\).*\[([\d.]+)(ms|s)\]$/);
        if (file && match) timings[file] += Number(match[1]) / (match[2] === "ms" ? 1000 : 1);
      }
    }
    if (!Object.keys(timings).length) throw new Error("no file timings found");
    await Bun.write(timingsPath, JSON.stringify(Object.fromEntries(Object.entries(timings).sort()), null, 2) + "\n");
    console.log(`Recorded ${Object.keys(timings).length} files with 0.1s/file loading allowance`);
  } else {
    const files = git("ls-files", "-z", "--cached", "--others", "--exclude-standard", "core").map(file => file.slice(5));
    let selection = { files: files.filter(file => testPattern.test(file)), reason: "full suite" };
    if (!args.includes("--full")) {
      let changed = args.flatMap((arg, index) => arg === "--changed" ? [args[index + 1]] : []);
      if (!changed.length) {
        const base = value("--base", process.env.CI_BASE_SHA ?? "");
        if (!base) throw new Error("provide --base, --changed or --full");
        const mergeBase = execFileSync("git", ["merge-base", base, "HEAD"], { cwd: root, encoding: "utf8" }).trim();
        changed = git("diff", "--name-only", "--no-renames", "-z", mergeBase, "HEAD");
      }
      if (!changed.some(file => file.startsWith(".github/workflows/") || file.startsWith("tools/ci/") || file.startsWith("migration/disabled-workflows/core/"))) {
        try {
          const { graph, unsafe } = importGraph(files);
          selection = selectTests(files, changed.filter(file => file.startsWith("core/")).map(file => file.slice(5)), graph, unsafe);
          if (unsafe.size) console.log(`Always include dependents of ${unsafe.size} files with computed/unresolved loading`);
        } catch (error) {
          console.log(`Full suite: graph unavailable: ${error}`);
        }
      }
    }
    const shards = balance(selection.files, JSON.parse(readFileSync(timingsPath, "utf8")), Number(value("--shards", "4")));
    console.log(`${selection.reason}: ${selection.files.length} files`);
    shards.forEach((shard, index) => console.log(`shard ${index + 1}: ${shard.files.length} files, estimated ${shard.seconds.toFixed(3)}s`));
    if (args.includes("--run")) {
      const shard = shards[Number(value("--shard", "1")) - 1];
      if (!shard) throw new Error("invalid shard");
      if (!shard.files.length) console.log("Empty shard; nothing to run");
      else {
        const result = Bun.spawnSync(["bun", "test", ...shard.files.map(file => `./${file}`)], { cwd: core, stdout: "inherit", stderr: "inherit", env: process.env });
        process.exit(result.exitCode);
      }
    }
  }
}
