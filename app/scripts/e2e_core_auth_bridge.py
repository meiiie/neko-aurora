"""Real compiled auth-bridge journey on owned synthetic sources and loopback only."""
import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import threading
import time
import traceback
import urllib.parse
from http.server import ThreadingHTTPServer

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'scripts'))
from e2e_principal_device import Client, ready
from e2e_tag_collaboration_daily import CoreHandle
from fixture_provider import Handler as BaseHandler, admitted_fixture_context


class AuthModel(BaseHandler):
    secret = 'fixture-auth-environment-canary'
    secondary = 'fixture-auth-refresh-canary'
    release = threading.Event()
    reject_release = threading.Event()
    lock = threading.Lock()
    observations = []
    oauth = False
    no_key = False

    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        try:
            body = json.loads(raw)
        except ValueError:
            body = {}
        manifest = admitted_fixture_context(body.get('messages', []))
        text = manifest['primaryMessage']['text'] if manifest else ''
        match = self.headers.get('Authorization') is None if self.no_key else self.headers.get('Authorization') == 'Bearer ' + self.secret
        identity = self.headers.get('X-Msh-Platform') == 'kimi_code_cli' and bool(self.headers.get('X-Msh-Device-Id'))
        with self.lock:
            self.observations.append({'inferencePath': self.path == '/v1/chat/completions', 'credentialMatches': match, 'modelMatches': body.get('model') == 'auth-fixture-model', 'identityHeadersMatch': bool(identity), 'hasManifest': manifest is not None, 'inputBytes': len(raw), 'maxTokens': body.get('max_tokens'), 'unauthorized': '[auth:401]' in text, 'held': '[auth:hold]' in text})
        if self.path != '/v1/chat/completions' or not match or manifest is None or body.get('model') != 'auth-fixture-model' or self.oauth and not identity:
            self.send_error(403)
            return
        if '[auth:401]' in text:
            self.reject_release.wait(20)
            self.send_response(401)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Connection', 'close')
            self.end_headers()
            self.wfile.write(json.dumps({'error': self.secret + self.secondary}).encode())
            return
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Connection', 'close')
        self.end_headers()
        try:
            self.chunk({'role': 'assistant', 'content': 'AUTH_FIXTURE_ACCEPTED '})
            if '[auth:echo]' in text:
                for secret in [self.secret, self.secondary] if self.oauth else [self.secret]:
                    for piece in [secret[:5], secret[5:17], secret[17:]]:
                        self.chunk({'content': piece})
            if '[auth:hold]' in text:
                self.wfile.flush()
                self.release.wait(40)
            self.chunk({'content': 'bounded Core-only auth.'})
            self.chunk({}, 'stop')
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


def digest(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    for arg in ['go', 'host', 'core', 'go-tool', 'output']:
        parser.add_argument('--' + arg, required=True, type=pathlib.Path)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--mode', required=True, choices=['env', 'kimi', 'file', 'none'])
    parser.add_argument('--admission-rounds', type=int, default=0, choices=range(17))
    args = parser.parse_args()
    out = args.output.resolve()
    assert out.is_relative_to(pathlib.Path('E:/CodexScratch/core-auth-bridge-20261012/root').resolve())
    out.mkdir(parents=True, exist_ok=False)
    private = out / 'private-fixture'
    private.mkdir()
    home = private / 'source-home'
    (home / '.neko-core').mkdir(parents=True)
    user = private / 'service-home'
    user.mkdir()
    state = private / 'state'
    AuthModel.oauth = args.mode == 'kimi'
    AuthModel.no_key = args.mode == 'none'
    AuthModel.observations = []
    AuthModel.release.clear()
    AuthModel.reject_release.clear()
    server = ThreadingHTTPServer(('127.0.0.1', 0), AuthModel)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    origin = 'http://127.0.0.1:%d' % server.server_address[1]
    source_file = home / '.neko-core/config.json'
    auth_file = home / '.neko-core/kimi-auth.json'
    profile = {'provider': 'kimi' if AuthModel.oauth else 'openai_compat', 'model': 'auth-fixture-model', 'base_url': origin + '/v1', 'auth': 'kimi_oauth' if AuthModel.oauth else 'api_key'}
    if args.mode == 'env':
        profile['key_env'] = 'FIXTURE_CORE_AUTH_KEY'
    elif args.mode == 'file':
        profile['api_key'] = AuthModel.secret
    elif AuthModel.no_key:
        profile['auth'] = 'none'
    source_file.write_text(json.dumps({'active_profile': 'auth-owned-fixture', 'profiles': {'auth-owned-fixture': profile}}), encoding='utf-8')
    if AuthModel.oauth:
        AuthModel.secret = 'fixture-cached-kimi-access-canary'
        auth_file.write_text(json.dumps({'access_token': AuthModel.secret, 'refresh_token': AuthModel.secondary, 'expires_at': int(time.time()) + 3900, 'token_type': 'Bearer'}), encoding='utf-8')
    source_hash = digest(source_file)
    auth_hash = digest(auth_file) if auth_file.exists() else None
    config = private / 'trusted-sources.json'
    source_descriptor = {'id': 'auth-core-fixture', 'label': 'Synthetic Core auth source', 'kind': 'fixture', 'credentialHome': str(home), 'providerOrigin': origin}
    if args.mode == 'env':
        source_descriptor['environmentBroker'] = True
    config.write_text(json.dumps({'version': 1, 'hostExecutable': str(args.host), 'coreExecutable': str(args.core), 'supervisor': str(ROOT / 'scripts/core_supervisor.py'), 'sources': [source_descriptor]}), encoding='utf-8')
    common = ['SYSTEMROOT', 'WINDIR', 'PATH', 'PATHEXT', 'COMSPEC', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS']
    env = {name: os.environ[name] for name in common if name in os.environ}
    env.update({'USERPROFILE': str(user), 'HOME': str(user), 'TEMP': str(private), 'TMP': str(private), 'PYTHONDONTWRITEBYTECODE': '1'})
    if args.mode == 'env':
        env['FIXTURE_CORE_AUTH_KEY'] = AuthModel.secret
        env['FIXTURE_UNSELECTED_KEY'] = 'fixture-unselected-key-never-forwarded'
    checks = []
    report = {'status': 'RUNNING', 'applicationCommit': args.commit, 'mode': args.mode, 'coreSHA256': digest(args.core), 'hostSHA256': digest(args.host), 'controlSHA256': digest(args.go), 'runnerSHA256': digest(__file__), 'command': [sys.executable, *sys.argv], 'checks': checks, 'scope': 'Exact compiled Go/Rust/Core, synthetic env/file key/cached Kimi token or no-key and loopback inference only. No actual profiles, environment credentials, login/refresh, provider, native GUI, candidate8767 or live8765.'}
    service = None
    handles = []
    log = (private / 'service.log').open('x', encoding='utf-8')

    def check(name, value):
        checks.append({'name': name, 'status': 'PASS' if value else 'FAIL'})
        assert value, name

    def start():
        process = subprocess.Popen([str(args.go), '--listen', '127.0.0.1:0', '--state', str(state), '--host-executable', str(args.host), '--core-profile-config', str(config)], stdout=subprocess.PIPE, stderr=log, text=True, env=env, creationflags=subprocess.CREATE_NO_WINDOW)
        client = Client(ready(process))
        client.opener.open(client.endpoint + '/').read()
        return process, client

    def snapshot():
        return client.request('/v1/snapshot')

    def inspect(nonce):
        source = client.request('/v1/core-profile-sources')['sources'][0]
        consent = client.request('/v1/core-profile-inspection-consents', {'sourceId': source['id'], 'expectedSourceRevision': source['revision'], 'approveInspection': True, 'ttlSeconds': 300, 'clientNonce': nonce})['inspectionConsent']
        metadata = client.request('/v1/core-profiles?' + urllib.parse.urlencode({'sourceId': source['id'], 'inspectionConsentId': consent['id']}))
        selected = next(p for p in metadata['profiles'] if p['id'] == 'auth-owned-fixture')
        return source, metadata, selected

    def approve(source, profile, nonce, seconds=90):
        body = {'sourceId': source['id'], 'expectedSourceRevision': source['revision'], 'profileId': profile['id'], 'profileRevision': profile['profileRevision'], 'provider': profile['provider'], 'model': profile['model'], 'providerOrigin': profile['providerOrigin'], 'channelId': 'conversation-bot-meiiie', 'expectedChannelRevision': 1, 'botIds': ['bot-meiiie'], 'maxInputBytes': 64000, 'maxOutputTokens': 4096, 'maxRequests': 2, 'maxSteps': 2, 'maxSeconds': seconds, 'ttlSeconds': 300, 'approveExecution': True, 'clientNonce': nonce}
        return client.request('/v1/core-profile-consents', body)['consent']

    def submit(consent, nonce, text='[auth:fast] Bounded synthetic auth request', code=201):
        body = {'botId': 'bot-meiiie', 'channelId': 'conversation-bot-meiiie', 'expectedChannelRevision': 1, 'lane': 'foreground', 'text': text, 'clientNonce': nonce, 'coreProfileSelection': {'consentId': consent['id'], 'expectedConsentRevision': consent['revision']}}
        return client.request('/v1/messages', body, code=code)['work']

    def held(consent, nonce, text):
        work = submit(consent, nonce, text)
        running = client.wait(work['requestId'], lambda w: w['status'] == 'running' and w.get('core', {}).get('nativeProcessId', 0) > 0 and 'AUTH_FIXTURE_ACCEPTED' in w.get('reply', ''), timeout=35)
        handle = CoreHandle(running['core']['nativeProcessId'], args.core)
        handles.append(handle)
        check('genuine live native Core HANDLE: ' + nonce, not handle.stopped())
        return work, handle

    try:
        head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip()
        build = subprocess.check_output([str(args.go_tool), 'version', '-m', str(args.go)]).decode()
        check('clean exact application control build', head == args.commit and not subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT) and 'vcs.revision=' + args.commit in build and 'vcs.modified=false' in build)
        service, client = start()
        discovery = client.request('/v1/core-profile-sources')
        check('discovery neither exposes source home nor creates capsule/inference', str(home) not in json.dumps(discovery) and not AuthModel.observations and not list((state / 'core-profile-bridge').rglob('environment.capsule')))
        client.request('/v1/core-profiles?' + urllib.parse.urlencode({'sourceId': discovery['sources'][0]['id'], 'inspectionConsentId': 'missing-owned'}), code=403)
        source, metadata, profile = inspect('auth-inspection-original')
        credential_source = {'env': 'process_env', 'kimi': 'oauth_cache', 'file': 'file_api_key', 'none': 'local_none'}[args.mode]
        check('Core v2 catalog supports exact bounded auth tuple', metadata['version'] == 2 and profile['bridgeTransport'] == 'supported' and profile['profileRevision'].startswith('pr2_') and profile['credentialSource'] == credential_source and profile['authPolicy'] == ('cached_token_no_refresh' if AuthModel.oauth else 'static_key') and profile['credentialReadiness'] == 'not_checked')
        public = json.dumps(metadata)
        check('metadata contains no credential/env name/private path and invokes no model', all(v not in public for v in [AuthModel.secret, AuthModel.secondary, 'FIXTURE_CORE_AUTH_KEY', str(home)]) and not AuthModel.observations)
        check('inspection leaves original synthetic config/auth files unchanged', digest(source_file) == source_hash and (not AuthModel.oauth or digest(auth_file) == auth_hash))
        if args.mode == 'env':
            capsules = list((state / 'core-profile-bridge').rglob('environment.capsule'))
            check('env captured only in encrypted private capsule after consent', len(capsules) == 1 and AuthModel.secret.encode() not in capsules[0].read_bytes() and profile['credentialSnapshotId'] == metadata['sourceSnapshot']['id'])
        consent = approve(source, profile, 'auth-execution-original')
        check('consent respects original limits and credential deadline without inference', consent['expiresAtUnixMs'] == min(consent['createdAtUnixMs'] + 300000, profile.get('credentialExpiresAtUnixMs', float('inf'))) and not AuthModel.observations)
        work = submit(consent, 'auth-request-original')
        completed = client.wait(work['requestId'], lambda w: w['status'] == 'completed', timeout=40)
        check('real native Core completes selected env/cached-token request', 'AUTH_FIXTURE_ACCEPTED' in completed['reply'] and completed.get('core', {}).get('nativeProcessId', 0) > 0)
        check('loopback sees exact token/model/context and OAuth identity only', len(AuthModel.observations) == 1 and AuthModel.observations[0]['credentialMatches'] and AuthModel.observations[0]['modelMatches'] and AuthModel.observations[0]['hasManifest'] and (not AuthModel.oauth or AuthModel.observations[0]['identityHeadersMatch']))
        duplicate = submit(consent, 'auth-request-original', code=200)
        check('same nonce preserves identity and budget without provider replay', duplicate['requestId'] == completed['requestId'] and len(AuthModel.observations) == 1)
        pin = completed['coreProfilePin']
        check('public Pin2 binds auth policy/snapshot/expiry without secret/path', pin['version'] == 2 and pin['authPolicy'] == profile['authPolicy'] and pin.get('credentialSnapshotId') == profile.get('credentialSnapshotId') and pin.get('credentialExpiresAtUnixMs') == profile.get('credentialExpiresAtUnixMs') and all(v not in json.dumps(pin) for v in [AuthModel.secret, AuthModel.secondary, str(home)]))
        report['admissionRounds'] = args.admission_rounds
        for number in range(args.admission_rounds):
            extra = submit(consent, 'auth-request-repeat-' + str(number), '[auth:fast] Repeat bounded admission ' + str(number))
            client.wait(extra['requestId'], lambda w: w['status'] == 'completed', timeout=35)
            check('unchanged scope repeated admission ' + str(number), len(AuthModel.observations) == number + 2)
        echo = submit(consent, 'auth-request-echo', '[auth:fast] Local no-key request' if AuthModel.no_key else '[auth:echo] Provider emits split synthetic credential values')
        echoed = client.wait(echo['requestId'], lambda w: w['status'] == 'completed', timeout=40)
        public = json.dumps(snapshot()) + (state / 'transactions.jsonl').read_text(encoding='utf-8')
        check('both split access/key and refresh token remain absent from public output', 'AUTH_FIXTURE_ACCEPTED' in echoed['reply'] and AuthModel.secret not in public and AuthModel.secondary not in public)
        if AuthModel.oauth:
            failed = submit(consent, 'auth-request-401', '[auth:401] Cached token rejection must never refresh')
            running = client.wait(failed['requestId'], lambda w: w['status'] == 'running' and w.get('core', {}).get('nativeProcessId', 0) > 0, timeout=15)
            handle = CoreHandle(running['core']['nativeProcessId'], args.core)
            handles.append(handle)
            AuthModel.reject_release.set()
            interrupted = client.wait(failed['requestId'], lambda w: w['status'] == 'recovery_required' and w.get('runtimeStopped') is True, timeout=35)
            check('401 preserves non-success recovery state and stops genuine Core without automatic retry', interrupted['failure'] == 'core_rpc_rejected' and handle.stopped())
            check('401 causes one terminal inference attempt, no refresh route or source write', len(AuthModel.observations) == 3 + args.admission_rounds and sum(o['unauthorized'] for o in AuthModel.observations) == 1 and all(o['inferencePath'] for o in AuthModel.observations) and digest(auth_file) == auth_hash)
            public = json.dumps(snapshot()) + (state / 'transactions.jsonl').read_text(encoding='utf-8')
            check('provider error cannot publish access/refresh token values', AuthModel.secret not in public and AuthModel.secondary not in public)
        elif args.mode == 'env':
            before = len(AuthModel.observations)
            pending, handle = held(consent, 'auth-request-new-snapshot', '[auth:hold] New inspection retires old env snapshot')
            _, fresh_metadata, fresh_profile = inspect('auth-inspection-new-snapshot')
            suspended = client.wait(pending['requestId'], lambda w: w['status'] == 'suspended' and w.get('runtimeStopped') is True, timeout=25)
            check('explicit new snapshot retires original pin and stops genuine Core', fresh_profile['credentialSnapshotId'] != profile['credentialSnapshotId'] and handle.stopped() and suspended['failure'] == 'core_profile_authority_changed')
            check('old env scope cannot silently rebind new snapshot', suspended['coreProfilePin']['credentialSnapshotId'] == pin['credentialSnapshotId'] and len(AuthModel.observations) == before + 1)
            profile = fresh_profile
            consent = approve(source, profile, 'auth-execution-after-snapshot')
        pending, handle = held(consent, 'auth-request-revoke', '[auth:hold] Explicit auth revoke')
        client.request('/v1/core-profile-consents/control', {'consentId': consent['id'], 'expectedRevision': consent['revision'], 'action': 'revoke', 'clientNonce': 'auth-revoke-original'})
        suspended = client.wait(pending['requestId'], lambda w: w['status'] == 'suspended' and w.get('runtimeStopped') is True, timeout=25)
        check('explicit revoke fences output/effects and produces genuine stop', handle.stopped() and suspended['coreProfilePin']['consentRevision'] == 1)
        receipt = client.request('/v1/core-profile-receipts?' + urllib.parse.urlencode({'kind': 'execution', 'clientNonce': 'auth-execution-after-snapshot' if args.mode == 'env' else 'auth-execution-original'}))
        check('receipt remains historical and cannot resurrect revoked scope', receipt['found'] and not receipt['active'] and receipt['consent']['revision'] == 1)
        source, metadata, profile = inspect('auth-inspection-expiry')
        deadline = approve(source, profile, 'auth-execution-deadline', seconds=15)
        pending, handle = held(deadline, 'auth-request-deadline', '[auth:hold] Bound request auth deadline')
        suspended = client.wait(pending['requestId'], lambda w: w['status'] == 'suspended' and w.get('runtimeStopped') is True, timeout=30)
        check('bound deadline stops actual Core without renewing credential/scope', handle.stopped() and suspended['coreProfilePin']['expiresAtUnixMs'] <= int(time.time() * 1000))
        AuthModel.release.set()
        before = len(AuthModel.observations)
        time.sleep(.1)
        check('late provider release cannot publish stopped output or spend again', len(AuthModel.observations) == before and next(w for w in snapshot()['work'] if w['requestId'] == pending['requestId'])['status'] == 'suspended')
        service.terminate()
        service.wait(timeout=8)
        service, client = start()
        replay = next(w for w in snapshot()['work'] if w['requestId'] == completed['requestId'])
        check('restart restores original durable result without provider replay', replay['reply'] == completed['reply'] and len(AuthModel.observations) == before)
        if args.mode == 'env':
            current_source = client.request('/v1/core-profile-sources')['sources'][0]
            check('env service restart rotates source generation and requires fresh inspection', current_source['revision'] != source['revision'])
            client.request('/v1/messages', {'botId': 'bot-meiiie', 'channelId': 'conversation-bot-meiiie', 'expectedChannelRevision': 1, 'lane': 'foreground', 'text': 'Old env scope remains denied after restart', 'clientNonce': 'auth-old-scope-after-restart', 'coreProfileSelection': {'consentId': deadline['id'], 'expectedConsentRevision': 1}}, code=409)
            check('restart does not reuse original env credential authority', len(AuthModel.observations) == before)
        check('synthetic source config/auth files still unchanged at end', digest(source_file) == source_hash and (not AuthModel.oauth or digest(auth_file) == auth_hash))
        report['status'] = 'PASS'
    except BaseException as error:
        report['status'] = 'FAIL'
        report['failureType'] = type(error).__name__
        report['failure'] = str(error)
        (out / 'failure.txt').write_text(traceback.format_exc(), encoding='utf-8')
    finally:
        AuthModel.release.set()
        AuthModel.reject_release.set()
        if service is not None and service.poll() is None:
            service.terminate()
            try:
                service.wait(timeout=8)
            except subprocess.TimeoutExpired:
                service.kill()
                service.wait(timeout=5)
        for handle in handles:
            handle.close()
        log.close()
        server.shutdown()
        server.server_close()
        report['observations'] = list(AuthModel.observations)
        report['ownedServiceStopped'] = service is None or service.poll() is not None
        (out / 'result.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report))
    return 0 if report['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
