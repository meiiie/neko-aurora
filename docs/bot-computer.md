# Bot computers (máy tính riêng cho từng bot)

Each Neko Aurora bot has its own Linux computer: a persistent home
(`state/aurora/computers/<botID>/home`, seen as `/home/bot` inside) plus a
sandbox that runs shell commands there. Package: `app/native/internal/computer`.

## Backends (`AURORA_COMPUTER`)
| value | backend | use |
|---|---|---|
| unset / `bwrap` | bubblewrap + prlimit (`bwrap.go`) | box, self-hosted VM |
| `railway` | Railway Sandboxes (`railway.go`) | production on Railway |
| `container:<rt>` | stub (`container.go`: Fly/Firecracker, gVisor, E2B notes) | future |
| `off` | disabled | |

### bwrap isolation
New user/pid/ipc/uts/net namespaces, read-only `/usr`, private `/tmp`, only the
bot's home bound read-write; no host home, `/workspace`, state dir or vault.
Environment built from constants (`--clearenv`), so `ZAI_API_KEY`, vault keys
and owner codes never enter. Limits: CPU 60 s, address space 1 GB, single file
64 MB, wall clock 60 s (kills the whole pid namespace), output 16 KB, home
quota 256 MB. Network off unless the bot's policy enables it.
Host-side file tools use `os.Root`, so symlinks planted in a home cannot
read or write outside it. Metadata (policy, command log) lives next to the
home, not inside it.

### Railway Sandboxes
Config: `RAILWAY_TOKEN` (or `RAILWAY_API_TOKEN`) + `RAILWAY_ENVIRONMENT_ID`.
GraphQL at `backboard.railway.com/graphql/v2`: `sandboxCreate` (ISOLATED,
1 vCPU/1 GB, idle 5 min, no variables), `sandbox` (poll to RUNNING),
`sandboxExec` (execHttp: 16 000 B/stream, ≤600 s), `sandboxDestroy` on reset.
Because Free/Trial VMs are destroyed after 5 idle minutes, the home stays on
our volume: pushed in as a base64 tarball before each command and pulled back
in 12 KB chunks after it (symlinks never cross). A destroyed VM is recreated
transparently. Caveats: Railway sandboxes always have outbound internet;
per-bot network-off is best effort there. Free plan: 10 running sandboxes per
environment, 2 vCPU/2 GB max. Upgrade path: the SDK's files WebSocket API and
checkpoints (pre-installed Python/pandas template).

## Harness tools
`computer_write_file`, `computer_read_file`, `computer_list` run directly (home
only). `computer_run` needs an approval card (kind `computer`) unless the bot's
"auto-run read-only" policy is on and the command is read-only (`ls`, `cat`,
`grep`, … no redirection/substitution). On approval the command runs, its
output appears on the card, and a hidden follow-up turn lets the bot continue.
All output is wrapped as untrusted data for the model.

## API
- `GET /app/api/agents/{id}/computer` status, policy, usage, files, last 30 commands
- `GET /app/api/agents/{id}/computer/file?path=` read a file (64 KB)
- `POST /app/api/agents/{id}/computer/policy` `{autoReadOnly, network}`
- `POST /app/api/agents/{id}/computer/reset` wipe home + log (and the Railway VM)

Backups skip `computers/*/home` (scratch disks).
