import argparse
import hashlib
import http.cookiejar
import json
import pathlib
import queue
import subprocess
import tempfile
import threading
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--commit', required=True)
args = parser.parse_args()
binary = pathlib.Path(args.binary).resolve()
repo = pathlib.Path(__file__).resolve().parent.parent
head = subprocess.check_output(['git', '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip()
if head != args.commit:
    raise SystemExit('Commit mismatch')
subprocess.run(['git', '-C', str(repo), 'diff', '--exit-code'], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['git', '-C', str(repo), 'diff', '--cached', '--exit-code'], check=True, stdout=subprocess.DEVNULL)

def start(directory):
    process = subprocess.Popen([str(binary), '--state', str(directory), '--listen', '127.0.0.1:0'], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    ready = queue.Queue()
    threading.Thread(target=lambda: ready.put(process.stdout.readline()), daemon=True).start()
    try:
        line = ready.get(timeout=10).strip()
        if not line.startswith('READY http://127.0.0.1:'):
            raise RuntimeError('Local server failed startup')
        return process, line.removeprefix('READY ')
    except BaseException:
        process.kill(); process.wait(timeout=10)
        raise

def client(base):
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    with opener.open(base, timeout=5):
        pass
    return opener

def request(opener, base, path, body=None):
    headers = {'Content-Type': 'application/json'} if body is not None else {}
    req = urllib.request.Request(base+path, data=json.dumps(body).encode() if body is not None else None, headers=headers)
    with opener.open(req, timeout=5) as response:
        return response.status, json.load(response)

checks = []
with tempfile.TemporaryDirectory(prefix='neko-e2e-') as state:
    process, base = start(state)
    try:
        first, second = client(base), client(base)
        background = dict(botId='bot-meiiie', clientNonce='e2e-background-nonce', text='Prepare the original report', lane='background')
        foreground = dict(botId='bot-meiiie', clientNonce='e2e-foreground-nonce', text='Independent follow-up', lane='foreground')
        status, bg = request(first, base, '/v1/messages', background)
        assert status == 201
        _, fg = request(second, base, '/v1/messages', foreground)
        assert bg['work']['requestId'] != fg['work']['requestId']
        assert bg['work']['sourceMessageId'] != fg['work']['sourceMessageId']
        checks.append('distinct_lane_and_message_identities')
        _, a = request(first, base, '/v1/snapshot')
        _, b = request(second, base, '/v1/snapshot')
        assert a == b and a['sequence'] == 2 and len(a['work']) == 2
        assert all(w['status'] == 'waiting_host' for w in a['work'])
        checks.append('two_clients_same_committed_history_and_honest_waiting_host')
        process.kill(); process.wait(timeout=10)
        process, base = start(state)
        first, second = client(base), client(base)
        status, replay = request(first, base, '/v1/messages', background)
        assert status == 200 and replay['duplicate'] and replay['work']['requestId'] == bg['work']['requestId']
        _, restored = request(second, base, '/v1/snapshot')
        assert restored == a
        checks.append('hard_kill_restart_preserves_acknowledged_history_without_duplicate_work')
        req = urllib.request.Request(base+'/v1/events?after=0', headers={'Last-Event-ID': '1'})
        with second.open(req, timeout=5) as response:
            assert response.readline() == b'id: 2\n'
        checks.append('sse_reconnect_uses_committed_sequence_cursor')
        _, cancelled = request(first, base, '/v1/cancel', {'requestId': bg['work']['requestId']})
        assert cancelled['status'] == 'cancelled' and cancelled['epoch'] == 2
        _, final = request(second, base, '/v1/snapshot')
        assert final['sequence'] == 3
        assert next(w for w in final['work'] if w['requestId'] == fg['work']['requestId'])['status'] == 'waiting_host'
        checks.append('cancel_target_preserves_foreground_obligation')
        try:
            request(first, base, '/v1/messages', {**background, 'text': 'silently replace the goal'})
            raise AssertionError('Nonce conflict accepted')
        except urllib.error.HTTPError as error:
            assert error.code == 409
        checks.append('conflicting_retry_is_rejected')
    finally:
        if process.poll() is None:
            process.kill(); process.wait(timeout=10)

print(json.dumps({'status':'pass', 'codeCommit':head, 'binarySha256':hashlib.sha256(binary.read_bytes()).hexdigest(), 'checks':checks, 'coreExecution':'not_configured_in_this_default_launch_test', 'cloudSync':'not_tested_local_only', 'phone':'not_tested'}, indent=2))
