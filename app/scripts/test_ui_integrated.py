import argparse
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
from e2e_permissions import OwnedCoreHandle

ROOT = pathlib.Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--commit', required=True)
    parser.add_argument('--scenario', choices=['redesign', 'context'], required=True)
    parser.add_argument('--tauri', action='store_true')
    args = parser.parse_args()
    assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip() == args.commit
    assert not subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip()
    environment = {key: os.environ[key] for key in ['SystemRoot', 'SystemDrive', 'WINDIR', 'ProgramData', 'ProgramFiles', 'ProgramFiles(x86)', 'ProgramW6432', 'CommonProgramFiles', 'TEMP', 'TMP', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA', 'PATH'] if key in os.environ}
    fixture = service = driver = None
    handles = {}
    os_proof = []
    output = ROOT / '.build' / ('evidence-native-context' if args.scenario == 'context' else 'evidence-redesign')
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='neko-integrated-ui-', dir=ROOT / '.build', ignore_cleanup_errors=True) as temporary:
        try:
            fixture = subprocess.Popen([sys.executable, str(ROOT / 'scripts/fixture_provider.py')], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, env=environment)
            ready = fixture.stdout.readline().strip()
            assert ready.startswith('READY ')
            config = pathlib.Path(temporary) / 'fixture.json'
            config.write_text(json.dumps({'hostExecutable': str(ROOT / 'host/target/debug/neko-host.exe'), 'coreExecutable': str(ROOT / '.build/core-windows/neko.exe'), 'supervisor': str(ROOT / 'scripts/core_supervisor.py'), 'fixtureEndpoint': ready[6:]}))
            service = subprocess.Popen([str(ROOT / '.build/neko-control.exe'), '--listen', '127.0.0.1:8765' if args.tauri else '127.0.0.1:0', '--state', str(pathlib.Path(temporary) / 'state'), '--host-executable', str(ROOT / 'host/target/debug/neko-host.exe'), '--core-fixture-config', str(config)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=environment)
            ready = service.stdout.readline().strip()
            assert ready.startswith('READY '), 'Owned local service did not start'
            script = ROOT / ('scripts/ui_native_context.mjs' if args.scenario == 'context' else 'scripts/ui_redesign_smoke.mjs')
            executable = str(ROOT / '.build/neko-desktop.exe') if args.tauri else r'C:\Program Files\Google\Chrome\Application\chrome.exe'
            command = [shutil.which('node'), str(script), executable, ready[6:], '--core'] + (['--tauri'] if args.tauri else [])
            if args.scenario == 'redesign':
                subprocess.run(command, cwd=ROOT, check=True, timeout=240, env=environment)
                report = json.loads((output / 'ui-smoke.json').read_text(encoding='utf-8'))
            else:
                driver = subprocess.Popen(command, cwd=ROOT, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=environment)
                report = None
                for line in driver.stdout:
                    event = json.loads(line)
                    if event['kind'] == 'capture_core':
                        pid = event['pid']
                        assert pid not in handles
                        handles[pid] = OwnedCoreHandle(pid)
                        os_proof.append({'pid': pid, 'phase': 'live_handle_bound_to_owned_Core_image', 'pass': True})
                        driver.stdin.write('{"ok":true}\n'); driver.stdin.flush()
                    elif event['kind'] == 'assert_stopped':
                        handles[event['pid']].assert_stopped()
                        os_proof.append({'pid': event['pid'], 'phase': 'same_kernel_handle_signalled_after_UI_revocation', 'pass': True})
                        driver.stdin.write('{"ok":true}\n'); driver.stdin.flush()
                    elif event['kind'] == 'report':
                        report = event['report']
                    else:
                        raise AssertionError('Unexpected owned UI proof event')
                exit_code = driver.wait(timeout=10)
                assert exit_code == 0 and report, driver.stderr.read()
                report['nativeProcessProof'] = os_proof
            report.update({'codeCommit': args.commit, 'workingTree': False, 'scenario': args.scenario, 'nativeTauri': args.tauri, 'coreFixture': 'actual pinned Windows Core with owned loopback synthetic provider; no live provider/grant', 'goBinarySha256': hashlib.sha256((ROOT / '.build/neko-control.exe').read_bytes()).hexdigest(), 'rustHostSha256': hashlib.sha256((ROOT / 'host/target/debug/neko-host.exe').read_bytes()).hexdigest(), 'coreSha256': hashlib.sha256((ROOT / '.build/core-windows/neko.exe').read_bytes()).hexdigest(), 'tauriBinarySha256': hashlib.sha256((ROOT / '.build/neko-desktop.exe').read_bytes()).hexdigest() if args.tauri else None, 'command': 'python scripts/test_ui_integrated.py --commit ' + args.commit + ' --scenario ' + args.scenario + (' --tauri' if args.tauri else ''), 'assetSha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in (ROOT / 'native/internal/web/assets').iterdir()}})
            report['visualAcceptance'] = 'rejected by user 2026-10-11; safety integration branch only; reconstruction from supplied reference pending'
            (output / 'ui-smoke.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
            assert not subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip()
            print(json.dumps({'codeCommit': args.commit, 'scenario': args.scenario, 'checks': len(report['checks']), 'status': report['status'], 'evidence': str(output / 'ui-smoke.json')}))
        finally:
            if driver and driver.poll() is None:
                driver.kill(); driver.wait(timeout=5)
            for handle in handles.values():
                handle.close()
            if service and service.poll() is None:
                service.kill(); service.wait(timeout=5)
            if fixture:
                fixture.stdin.close()
                try:
                    fixture.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    fixture.terminate(); fixture.wait(timeout=5)


if __name__ == '__main__':
    main()
