# Bot computers (máy tính riêng cho từng bot)

Each Neko Aurora bot has a private workspace: a persistent home
(`state/aurora/computers/<botID>/home`, seen as `/home/bot` inside) plus a
sandbox that runs shell commands there when configured. File creation, editing,
download and HTML previews do not require a running shell. Package:
`app/native/internal/computer`.

## Backends (`AURORA_COMPUTER`)
| value | backend | use |
|---|---|---|
| unset on Windows / `files` | persistent files, no command runner | local file creation and HTML preview |
| unset on Linux / `bwrap` | bubblewrap + prlimit (`bwrap.go`) | box, self-hosted VM |
| `docker` | local Linux Docker engine (`docker.go`) | isolated local coding on Windows/Linux |
| `railway` | Railway Sandboxes (`railway.go`) | production on Railway |
| `container:<rt>` | stub (`container.go`: Fly/Firecracker, gVisor, E2B notes) | future |
| `off` | disabled | |

### Local coding with Docker

With a local Linux Docker engine running, build the dependency image from the
repository root:

```powershell
docker build -t neko-aurora-computer:local app/computer
$env:AURORA_COMPUTER = 'docker'
```

Then start Aurora with the existing local launcher. Node.js/npm, Python 3 and
Git are available inside the image. `AURORA_COMPUTER_IMAGE` selects a different
locally installed image. Execution never pulls images or starts a remote daemon.
Windows uses Docker Desktop's local Linux named pipe, Linux the local Unix socket;
`DOCKER_HOST` and cloud contexts are not inherited.

Every command runs in a fresh non-root container with a read-only root filesystem,
all capabilities dropped, no privilege escalation, CPU/memory/process/file/output
limits and a wall-clock timeout. Only that bot's home is mounted. Host credentials,
source checkouts and Docker's socket are not passed into the container. Network is
off unless the bot's existing policy allows it. The container is removed after
execution and on timeout; only the workspace files persist. Dependencies installed
in `/tmp` do not survive another command. No dev-server ports are exposed.

If Docker or the image is unavailable, the status describes the setup problem and
file tools remain usable. An explicit `readonly` policy still blocks file writes.
The model transport's native read-only sandbox is separate from Aurora's workspace
tools; Aurora tells the model the actual capabilities for each turn.

Connected providers use Core's `bin/aurora-completion.ts` adapter. Aurora passes
structured system/user/assistant messages over bounded JSON stdin, retaining the
host tool protocol at system authority. This path does not construct Core's
terminal agent, load its local-workspace instructions, or enable its host tools.
Core still owns provider selection, account authentication and transport. Aurora
validates the returned directives and executes them through the bot's existing
workspace policy. Long HTML and Unicode history never enter Windows command-line
arguments. Provider failures expose bounded error codes, and truncated output is
marked for the existing file-recovery flow.

### HTML previews

An HTML download link in chat also offers **Preview**. HTML files in the bot's
workspace open the same preview, with a separate download action. Use a self-contained
UTF-8 HTML file with inline CSS/JavaScript/SVG. External scripts, styles, images,
fonts, fetch requests, form destinations and nested frames are blocked. Local
JavaScript form handlers can prevent submission and update the page. This is a document
preview, not a deployed website or a proxy to a development server.

The response and iframe both impose an opaque sandbox origin (scripts and form events allowed,
same-origin access forbidden). Preview code cannot read Aurora cookies, storage,
parent DOM or APIs. Each request checks the current workspace and bot; only HTML
up to 4 MiB is accepted. Downloads retain their attachment-only behavior.

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
- Status distinguishes `fileAccess`, `canWrite` and `available` (command runner),
  with a stable `setupCode`. Disabled computers return a status object, not HTTP 503.
- `GET /app/api/agents/{id}/computer/file?path=` read a file (64 KB)
- `GET /app/api/agents/{id}/computer/preview?path=&workspaceId=` isolated HTML preview
- `POST /app/api/agents/{id}/computer/policy` `{autoReadOnly, network}`
- `POST /app/api/agents/{id}/computer/reset` wipe home + log (and the Railway VM)

Backups skip `computers/*/home` (scratch disks).
