"""Check runtime rejection before launch and copied historical journal replay."""
import argparse, hashlib, json, pathlib, shutil, subprocess, tempfile
from e2e_core import ROOT, Client

def sha(path):
    with path.open('rb') as file:
        return hashlib.file_digest(file, 'sha256').hexdigest()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--commit', required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    parser.add_argument('--historical-core', type=pathlib.Path, required=True)
    parser.add_argument('--historical-state', type=pathlib.Path, required=True)
    args = parser.parse_args()
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    assert head == args.commit and not subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT)
    pins = json.loads((ROOT/'native/internal/protocol/core-runtimes-v1.json').read_text())
    host = ROOT/'host/target/debug/neko-host.exe'
    core = ROOT/'.build/core-windows/neko.exe'
    assert sha(core) == pins['windows_x64']['sha256']
    assert sha(args.historical_core) == pins['windows_x64_previous_writer_v2']['sha256']
    args.output.parent.mkdir(parents=True, exist_ok=True)
    scratch = pathlib.Path(tempfile.mkdtemp(prefix='owned-adoption-', dir=args.output.parent))
    checks = []
    process = None
    try:
        for label, binary, resume, expected in [
            ('old_executable_denied_before_task_creation', args.historical_core, None, 'runtime_digest_mismatch'),
            ('old_saved_binding_denied_before_task_creation', core,
             {'coreSha256': pins['windows_x64_previous_writer_v2']['sha256']}, 'resume_runtime_changed'),
        ]:
            state = scratch/label
            state.mkdir()
            spec = {'requestId': 'req-'+'a'*32, 'text': 'Fresh bounded request'}
            if resume is not None:
                spec['resume'] = resume
            result = subprocess.run([str(host), '--runtime', str(binary), '--supervisor', str(ROOT/'scripts/core_supervisor.py'),
                                     '--state-root', str(state), '--fixture-endpoint', 'http://127.0.0.1:1/v1'],
                                    input=json.dumps(spec)+'\n', text=True, capture_output=True, timeout=20)
            (scratch/(label+'.stderr')).write_text(result.stderr, encoding='utf-8')
            assert result.returncode == 1 and result.stderr.strip() == expected, (label, result.returncode, result.stderr)
            assert result.stdout == '' and not list(state.iterdir()), label
            checks.append({'name': label, 'pass': True, 'code': expected})
        state = scratch/'historical-replay'
        state.mkdir()
        original = args.historical_state/'transactions.jsonl'
        original_hash = sha(original)
        shutil.copyfile(original, state/'transactions.jsonl')
        for name in ['uploads', 'artifacts']:
            source = args.historical_state/name
            if source.exists():
                shutil.copytree(source, state/name)
        previous = None
        old_digest = pins['windows_x64_previous_writer_v2']['sha256']
        for index in range(3):
            process = subprocess.Popen([str(ROOT/'.build/neko-aurora.exe'), '--listen', '127.0.0.1:0', '--state', str(state),
                                        '--host-executable', str(host)], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, text=True)
            line = process.stdout.readline().strip()
            assert line.startswith('READY '), 'Historical journal did not open'
            client = Client(line[6:])
            snapshot = client.request('/v1/snapshot')
            historical = [work for work in snapshot['work'] if work.get('core', {}).get('coreSha256') == old_digest]
            assert historical, 'No old runtime provenance in fixture journal'
            if previous is not None:
                assert snapshot['work'] == previous, 'Historical pins or state changed on restart'
            else:
                added = client.send('Independent fresh intake survives historical replay', nonce='core-adoption-unrelated-intake')
                assert added['status'] == 'waiting_host' and not added.get('core')
                previous = client.request('/v1/snapshot')['work']
            process.kill()
            process.wait(timeout=5)
            assert not process.stderr.read().strip(), 'Historical replay service emitted error'
            process = None
        assert sha(original) == original_hash, 'Read-only source journal mutated'
        checks.append({'name': 'three_restarts_preserve_old_runtime_pins_and_fresh_intake', 'pass': True,
                       'historicalJournalSha256': original_hash, 'historicalWorkCount': len(historical)})
        result = {'commit': head, 'workingTree': False, 'status': 'PASS', 'scratch': str(scratch), 'checks': checks,
                  'hostSha256': sha(host), 'coreSha256': sha(core), 'goSha256': sha(ROOT/'.build/neko-aurora.exe'),
                  'scope': 'Actual Rust boundary and copied writer-v2 journal; no user-state migration, automatic binding adoption or real provider.'}
    except BaseException as error:
        result = {'commit': head, 'status': 'FAIL', 'scratch': str(scratch), 'checks': checks,
                  'failure': type(error).__name__+': '+str(error)[:1200]}
        args.output.write_text(json.dumps(result, indent=2)+'\n', encoding='utf-8')
        raise
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait(timeout=5)
    args.output.write_text(json.dumps(result, indent=2)+'\n', encoding='utf-8')
    print(json.dumps(result, indent=2))

if __name__ == '__main__':
    main()
