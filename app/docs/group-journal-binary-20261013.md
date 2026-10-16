# Candidate executable: genuine group journal v1/v2 proof

Candidate `E:/CodexScratch/parallel-product-20261013/candidate-5349523` contains the actual control-plane executable supporting the report-context Version2 reader/writer; this is not merely a source-only note. Binary SHA256 `c2732194124fb989dc84b301fb06e910f64579e94f79e930bf57b1b91ffa337d`, VCS `53495231a06aac97c1092029fda35971bdbc08c5`, `vcs.modified=false`.

Command from Root source checkout: `C:/Python313/python.exe scripts/e2e_group_journal_binary_20261013.py --output E:/CodexScratch/group-journal-binary-20261013/actual-v1-v2-r3`.

Windows amd64/Python3.13; named Windows environment keys only, with TEMP/TMP/USERPROFILE redirected to an owned fixture. Old binary is candidate-c91e4de control-plane SHA256 `a7663f40fb0d3893f64e1fac8f933d42672812228b6ffb7837665d01bb6ecd03`. No working Go source was compiled while other workers wrote files; both executables were pinned independently of the dirty source checkout.

**PASS16/16**, exact command/binary/runner hashes and checks: `E:/CodexScratch/group-journal-binary-20261013/actual-v1-v2-r3/results.json`.

- Current binary accepts real HTTP group intake into fresh state; its actual journal contains `publishedReportContext.version=2`. No task is running.
- Same executable restarts from that journal, accepts the original duplicate nonce with the same group identity, and keeps the original frozen policy pin unchanged.
- Old binary refuses a copy of the Version2 journal before READY, returns its coarse safe public error `Local store unavailable; inspect state before retry`, and leaves the copied journal bytes unchanged.
- Old executable creates a separate genuine Version1 journal by HTTP. Current binary opens it, returns the original duplicate group identity and retains its Version1 frozen policy.

Five owned Go processes were stopped. Journals/logs remain in the fixture's private directory. Live8765, candidate user state, installed Core, archives, secrets/profile settings and other processes were not accessed or changed. Neither Core nor Rust host/browser/provider ran. Group execution, report-filled pin composition and native UI remain separate gates; this proof exercises format/intake/replay/identity with a simple authored group.

Earlier `actual-v1-v2-r1` and `r2` are retained harness oracle failures: the script incorrectly required the hidden internal `journal_corrupt` error string, while the CLI deliberately reports only the coarse safe message. Both rounds already observed the intended pre-READY refusal and pinned current-binary Version2 creation/restart. Final assertion checks the actual public contract; no product code or assertion about internal error cause was changed to obtain a pass.

## Actual V3 executable

Clean code5d405b723e45501c12054a8733f103c925ac8553 control93866214935c44c533dac4d8904a56310a611974b61adab0c2d459a5a7ab3c83 passed23 real HTTP checks. Both pinned old binaries generate genuine V1/V2 journals; new reader preserves identity/nonce/frozen pins. New V3 restarts; old V1/V2 readers refuse copied V3 before READY without changing journal bytes. Eight owned processes closed, no Core/model/UI/live state. Exact command/environment/buildInfo/results: E:/CodexScratch/group-journal-binary-20261013/actual-v1-v2-v3-5d405b7/results.json and scripts/e2e_group_journal_v3_20261013.py. The previous16-check V1/V2 proof remains immutable.
