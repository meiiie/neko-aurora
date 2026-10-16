import { existsSync, lstatSync, realpathSync } from "node:fs";
import { isAbsolute, join, relative, resolve, sep } from "node:path";
import { homeDir } from "./home.ts";

export function credentialHomeDir(explicit = process.env.NEKO_CREDENTIAL_HOME): string {
  if (!explicit) return homeDir();
  if (!isAbsolute(explicit) || /[\x00-\x1f]/.test(explicit)) throw new Error("Invalid credential home");
  const root = resolve(explicit);
  if (!existsSync(root) || !lstatSync(root).isDirectory() || lstatSync(root).isSymbolicLink()) {
    throw new Error("Invalid credential home");
  }
  return realpathSync.native(root);
}

export function credentialPath(...parts: string[]): string {
  const home = credentialHomeDir();
  const path = join(home, ...parts);
  if (!process.env.NEKO_CREDENTIAL_HOME) return path;
  let current = home;
  for (const part of relative(home, path).split(sep)) {
    if (part === ".." || !part) throw new Error("Invalid credential path");
    current = join(current, part);
    if (existsSync(current) && lstatSync(current).isSymbolicLink()) throw new Error("Credential path alias refused");
  }
  return path;
}
