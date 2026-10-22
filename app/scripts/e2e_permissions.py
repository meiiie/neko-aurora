"""Local owner context revocation through actual Windows Core and owned fixture."""
import argparse
import ctypes
from ctypes import wintypes
import hashlib
import json
import pathlib
import subprocess
import sys
import tempfile
import time
import urllib.error
from e2e_core import ROOT, launch

class OwnedCoreHandle:
    def __init__(self, pid):
        self.kernel = ctypes.WinDLL('kernel32', use_last_error=True)
        self.kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
        self.kernel.OpenProcess.restype = wintypes.HANDLE
        self.kernel.QueryFullProcessImageNameW.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
        self.kernel.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
        self.kernel.WaitForSingleObject.restype = wintypes.DWORD
        self.kernel.CloseHandle.argtypes = [wintypes.HANDLE]
        self.handle = self.kernel.OpenProcess(0x101000, False, pid)
        assert self.handle, 'Owned native Core handle unavailable'
        image = ctypes.create_unicode_buffer(32768)
        size = wintypes.DWORD(len(image))
        assert self.kernel.QueryFullProcessImageNameW(self.handle, 0, image, ctypes.byref(size))
        assert pathlib.Path(image.value).resolve() == (ROOT / '.build/core-windows/neko.exe').resolve()
        assert self.kernel.WaitForSingleObject(self.handle, 0) == 258, 'Core was not live before revocation'

    def assert_stopped(self):
        assert self.kernel.WaitForSingleObject(self.handle, 0) == 0, 'Stop receipt preceded actual Core process exit'

    def close(self):
        self.kernel.CloseHandle(self.handle)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--commit')
    parser.add_argument('--output', type=pathlib.Path)
    args = parser.parse_args()
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    if args.commit:
        assert head == args.commit and not subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip()
    fixture = subprocess.Popen([sys.executable, str(ROOT / 'scripts/fixture_provider.py')], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    process = None
    checks = []
    handles = []
    try:
        ready = fixture.stdout.readline().strip()
        assert ready.startswith('READY ')
        with tempfile.TemporaryDirectory(prefix='neko-permission-e2e-', ignore_cleanup_errors=True) as directory:
            state = pathlib.Path(directory) / 'state'
            config = pathlib.Path(directory) / 'fixture.json'
            config.write_text(json.dumps({'hostExecutable': str(ROOT / 'host/target/debug/neko-host.exe'), 'coreExecutable': str(ROOT / '.build/core-windows/neko.exe'), 'supervisor': str(ROOT / 'scripts/core_supervisor.py'), 'fixtureEndpoint': ready[6:]}))
            process, client = launch(state, config)
            source = client.send('[fixture:foreground] Local source permission fixture.')
            client.wait(source['requestId'], lambda w: w['status'] == 'completed')
            active = client.send('[fixture:background] [fixture:context-inspect] Inspect the context for this obligation.', 'background')
            running = client.wait(active['requestId'], lambda w: bool(w.get('reply')))
            owned = OwnedCoreHandle(running['core']['nativeProcessId'])
            handles.append(owned)
            assert any(s['requestId'] == source['requestId'] for s in running['contextPin']['sources'])
            pin = running['contextPin']
            revoke = {'requestId': source['requestId'], 'expectedRevision': 0, 'granted': False, 'clientNonce': 'native-permission-revoke-1'}
            client.request('/v1/context-access', revoke)
            stopped = client.wait(active['requestId'], lambda w: w['status'] == 'suspended', timeout=25)
            owned.assert_stopped()
            assert stopped['desiredState'] == 'suspended' and stopped['epoch'] == active['epoch'] + 1
            assert stopped['runtimeStopped'] and stopped['runtimeStopReceipt']['nativeProcessId'] == running['core']['nativeProcessId']
            assert stopped['runtimeStopReceipt']['sessionId'] == running['core']['sessionId'] and stopped['contextPin'] == pin and stopped['core'] == running['core']
            assert not stopped.get('operations') and not stopped.get('artifacts')
            frozen_reply = stopped['reply']
            time.sleep(1)
            assert client.work(active['requestId'])['reply'] == frozen_reply
            checks.append({'name': 'revoke_stops_native_Core_and_fences_late_output_preserving_pin', 'pass': True})
            duplicate = client.request('/v1/context-access', revoke)
            assert duplicate['duplicate'] and duplicate['work']['contextAccess']['revision'] == 1
            try:
                client.request('/v1/context-access', dict(revoke, granted=True, clientNonce='native-permission-stale'))
                raise AssertionError('Stale revision admitted')
            except urllib.error.HTTPError as error:
                assert error.code == 409
            checks.append({'name': 'access_revision_CAS_and_nonce_idempotency', 'pass': True})
            fresh = client.request('/v1/context-refresh', {'requestId': active['requestId'], 'expectedEpoch': stopped['epoch']})
            assert fresh['contextPin']['sources'] == [] and fresh['contextRevision'] == 1 and fresh.get('core') is None
            assert fresh['contextAttempts'][0]['core'] == running['core'] and fresh['contextAttempts'][0]['reply'] == frozen_reply
            done = client.wait(active['requestId'], lambda w: w['status'] == 'completed', timeout=40)
            assert done['core']['sessionId'] != running['core']['sessionId']
            assert done['core']['task']['id'] != running['core']['task']['id']
            assert 'PIN_SHA=' + fresh['contextPin']['sha256'] in done['reply'] and 'PIN_SOURCES=[]' in done['reply']
            assert done['contextAttempts'][0]['reply'] == frozen_reply
            checks.append({'name': 'explicit_refresh_after_quiescence_uses_fresh_pin_and_archives_old_Core', 'pass': True})
            effect = client.send('[fixture:background] [fixture:artifact] File obligation stopped before tool dispatch.', 'background')
            effect_running = client.wait(effect['requestId'], lambda w: bool(w.get('reply')))
            owned_effect = OwnedCoreHandle(effect_running['core']['nativeProcessId'])
            handles.append(owned_effect)
            client.request('/v1/context-access', {'requestId': active['requestId'], 'expectedRevision': 0, 'granted': False, 'clientNonce': 'native-permission-effect-revoke'})
            effect_stopped = client.wait(effect['requestId'], lambda w: w['status'] == 'suspended', timeout=25)
            owned_effect.assert_stopped()
            assert not effect_stopped.get('operations') and not effect_stopped.get('artifacts') and effect_stopped['core'] == effect_running['core']
            checks.append({'name': 'revoke_before_Core_tool_denies_new_file_effect', 'pass': True})
            client.request('/v1/context-access', {'requestId': source['requestId'], 'expectedRevision': 1, 'granted': True, 'clientNonce': 'native-permission-regrant'})
            assert client.work(effect['requestId'])['status'] == 'suspended'
            checks.append({'name': 'regrant_does_not_automatically_resume_suspended_obligation', 'pass': True})
            process.kill()
            process.wait(timeout=5)
            process, client = launch(state, config)
            restored = client.work(effect['requestId'])
            assert restored == effect_stopped
            assert client.work(active['requestId'])['contextAttempts'] == done['contextAttempts']
            checks.append({'name': 'restart_preserves_suspended_fence_receipt_and_context_archive', 'pass': True})
        report = {'commit': head, 'workingTree': not bool(args.commit), 'scope': 'Native Windows Go/Rust/Core + owned loopback fixture. Local owner context reuse only; no cross-channel ACL, provider grants, model quality or phone acceptance. Explicit context refresh is a new context attempt after proven Core stop; it is not crash recovery.', 'goBinarySha256': hashlib.sha256((ROOT / '.build/neko-aurora.exe').read_bytes()).hexdigest(), 'rustHostSha256': hashlib.sha256((ROOT / 'host/target/debug/neko-host.exe').read_bytes()).hexdigest(), 'coreSha256': hashlib.sha256((ROOT / '.build/core-windows/neko.exe').read_bytes()).hexdigest(), 'checks': checks}
        if args.output:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
        print(json.dumps(report, indent=2))
    finally:
        for handle in handles:
            handle.close()
        if process and process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        fixture.stdin.close()
        try:
            fixture.wait(timeout=5)
        except subprocess.TimeoutExpired:
            fixture.terminate()

if __name__ == '__main__':
    main()
