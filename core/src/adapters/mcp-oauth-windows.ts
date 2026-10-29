import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { basename, dirname } from "node:path";
import { minimalWindowsSystemEnv, resolveWindowsSystemExecutable } from "../shared/windows-system.ts";

const script = String.raw`
$ErrorActionPreference='Stop'
try {
 [Console]::InputEncoding=[Text.UTF8Encoding]::new($false)
 Add-Type -AssemblyName System.Security
 $request=[Console]::In.ReadToEnd() | ConvertFrom-Json
 $bytes=[Convert]::FromBase64String($request.data)
 $entropy=[Convert]::FromBase64String($request.entropy)
 if ($request.op -eq 'protect') {
  $result=[Security.Cryptography.ProtectedData]::Protect($bytes,$entropy,[Security.Cryptography.DataProtectionScope]::CurrentUser)
  if (([IO.File]::GetAttributes($request.directory) -band [IO.FileAttributes]::ReparsePoint) -ne 0) {exit 1}
  $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
  $acl=New-Object Security.AccessControl.DirectorySecurity
  $acl.SetOwner($sid)
  $acl.SetAccessRuleProtection($true,$false)
  $rule=New-Object Security.AccessControl.FileSystemAccessRule($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow')
  $acl.AddAccessRule($rule)
  [IO.Directory]::SetAccessControl($request.directory,$acl)
  $actual=[IO.Directory]::GetAccessControl($request.directory)
  $rules=$actual.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])
  if (-not $actual.AreAccessRulesProtected -or $actual.GetOwner([Security.Principal.SecurityIdentifier]) -ne $sid -or $rules.Count -ne 1 -or $rules[0].IdentityReference -ne $sid -or $rules[0].AccessControlType -ne 'Allow' -or $rules[0].FileSystemRights -ne 'FullControl') {exit 1}
 } elseif ($request.op -eq 'unprotect') {
  $result=[Security.Cryptography.ProtectedData]::Unprotect($bytes,$entropy,[Security.Cryptography.DataProtectionScope]::CurrentUser)
 } else {exit 1}
 [Console]::Out.Write([Convert]::ToBase64String($result))
} catch {exit 1}
finally {
 if ($bytes) {[Array]::Clear($bytes,0,$bytes.Length)}
 if ($result) {[Array]::Clear($result,0,$result.Length)}
}
`;

export function cryptWindowsOAuth(path: string, bytes: Buffer, op: "protect" | "unprotect"): Buffer {
  const unavailable = () => new Error("MCP OAuth Windows credential protection is unavailable");
  const executable = resolveWindowsSystemExecutable("WindowsPowerShell\\v1.0\\powershell.exe");
  if (process.platform !== "win32" || !executable || bytes.length === 0 || bytes.length > (op === "protect" ? 65536 : 98304)) throw unavailable();
  const entropy = createHash("sha256").update(JSON.stringify(["neko-core-mcp-oauth-dpapi-v1", basename(dirname(path)), basename(path)])).digest("base64");
  // Only fixed code is in argv. Secrets travel through pipes, never a shell, environment or temporary file.
  const result = spawnSync(executable, ["-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script], {
    input: JSON.stringify({ op, data: bytes.toString("base64"), entropy, directory: dirname(path) }),
    env: { ...minimalWindowsSystemEnv(), PSModuleAnalysisCachePath: "NUL" }, windowsHide: true, timeout: 10_000, maxBuffer: 262144,
  });
  if (result.error || result.signal || result.status !== 0 || !result.stdout || result.stderr?.length) throw unavailable();
  const text = result.stdout.toString("ascii");
  const output = Buffer.from(text, "base64");
  if (!output.length || output.length > (op === "protect" ? 98304 : 65536) || output.toString("base64") !== text) { output.fill(0); throw unavailable(); }
  return output;
}
