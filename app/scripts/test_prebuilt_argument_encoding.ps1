param([Parameter(Mandatory=$true)][string]$SourceCommit, [Parameter(Mandatory=$true)][string]$OutputDirectory)
$ErrorActionPreference = "Stop"
if ($SourceCommit -notmatch '^[a-f0-9]{40}$') { throw "Exact source commit required" }
$repoDir = Split-Path -Parent $PSScriptRoot
$source = & git -C $repoDir show ($SourceCommit + ':scripts/run-prebuilt-local.ps1')
if ($LASTEXITCODE -ne 0) { throw "Missing source commit" }
$assignment = @($source | Where-Object { $_.StartsWith('$quotedArguments = ') })
if ($assignment.Count -ne 1) { throw "Expected exactly one trusted source assignment" }
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw "Use a fresh output folder" }
New-Item -ItemType Directory -Path $output | Out-Null
$argvScript = Join-Path $output "argv.mjs"
[IO.File]::WriteAllText($argvScript, 'console.log(JSON.stringify({argv:process.argv.slice(2)}));')
$checks = @()
$cases = @('E:\owned-fixture\state', 'E:\owned-fixture\state with spaces\', 'E:\') | ForEach-Object { [IO.Path]::GetFullPath($_) }
foreach ($statePath in $cases) {
    $arguments = @('--state', $statePath, '--listen', '127.0.0.1:8765', '--host-executable', 'E:\owned-fixture\neko-host.exe')
    . ([ScriptBlock]::Create($assignment[0]))
    $nodeArguments = @('"' + $argvScript + '"') + $quotedArguments
    $stdout = Join-Path $output ("argv-" + $checks.Count + ".json")
    $stderr = Join-Path $output ("argv-" + $checks.Count + ".stderr")
    $child = Start-Process -FilePath (Get-Command node).Source -ArgumentList $nodeArguments -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    $retainedHandle = $child.Handle
    $child.WaitForExit()
    $child.Refresh()
    if ($child.ExitCode -ne 0) { throw "Owned argument probe failed" }
    $observed = Get-Content -LiteralPath $stdout -Raw | ConvertFrom-Json
    $ok = @($observed.argv).Count -eq $arguments.Count
    for ($i=0; $ok -and $i -lt $arguments.Count; $i++) { $ok = $observed.argv[$i] -ceq $arguments[$i] }
    $checks += [ordered]@{ statePath=$statePath; status=$(if ($ok) { 'PASS' } else { 'FAIL' }); expected=$arguments; actual=$observed.argv }
}
$result = [ordered]@{sourceCommit=$SourceCommit; PowerShell=$PSVersionTable.PSVersion.ToString(); checks=$checks; scope='Actual hidden Node subprocess argument encoding only. No Go, desktop, state or provider started.'}
[IO.File]::WriteAllText((Join-Path $output 'validation.json'), ($result | ConvertTo-Json -Depth 6), [Text.Encoding]::UTF8)
if ($checks.Where({ $_.status -eq 'FAIL' }).Count) { throw "Native argument regression" }
Write-Output ("PASS " + $checks.Count + " native argument cases")
