"""Two isolated HTTP cookie jars against one owned loopback Go journal; no cloud/device claim."""
import argparse
import concurrent.futures
import hashlib
import http.cookiejar
import json
import os
import pathlib
import queue
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--commit', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
binary = pathlib.Path(args.binary).resolve()
output = pathlib.Path(args.output).resolve()
output.parent.mkdir(parents=True, exist_ok=True)
checks = []
environment = {k: os.environ[k] for k in ['SystemRoot', 'WINDIR', 'TEMP', 'TMP', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA'] if k in os.environ}


def start(directory):
    command = [str(binary), '--listen', '127.0.0.1:0', '--state', str(directory)]
    process = subprocess.Popen(command, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    ready = queue.Queue()
    threading.Thread(target=lambda: ready.put(process.stdout.readline()), daemon=True).start()
    try:
        line = ready.get(timeout=10).strip()
        if not line.startswith('READY http://127.0.0.1:'):
            raise RuntimeError('Owned loopback Go service failed startup')
        return process, line.removeprefix('READY '), command
    except BaseException:
        process.kill()
        process.wait(timeout=10)
        raise


def client(base):
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    with opener.open(base, timeout=5):
        pass
    return opener


def request(opener, base, route, body=None):
    headers = {'Content-Type': 'application/json'} if body is not None else {}
    req = urllib.request.Request(base + route, data=json.dumps(body).encode() if body is not None else None, headers=headers)
    try:
        with opener.open(req, timeout=5) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)


def check(name, ok, evidence):
    checks.append({'id': name, 'status': 'pass' if ok else 'fail', 'evidence': evidence})


started = time.time()
commands = []
process = None
try:
    with tempfile.TemporaryDirectory(prefix='neko-mobile-journal-') as directory:
        process, base, command = start(directory)
        commands.append(command)
        first, second = client(base), client(base)
        shared = dict(botId='bot-meiiie', clientNonce='mobile-shared-logical-request', text='Fixture original background goal', lane='background')
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda opener: request(opener, base, '/v1/messages', shared), [first, second]))
        ids = [body['work']['requestId'] for status, body in results]
        check('concurrent_same_bot_nonce_one_request', len(set(ids)) == 1 and sorted(status for status, _ in results) == [200, 201], {'statusCodes': [status for status, _ in results], 'uniqueRequestIds': len(set(ids))})
        status, conflict = request(second, base, '/v1/messages', {**shared, 'text': 'Fixture replacement goal'})
        check('same_nonce_changed_payload_rejects', status == 409, {'statusCode': status})
        status, independent = request(second, base, '/v1/messages', {**shared, 'clientNonce': 'mobile-distinct-new-request'})
        check('different_nonce_same_text_is_independent', status == 201 and independent['work']['requestId'] != ids[0], {'statusCode': status, 'newRequest': independent['work']['requestId'] != ids[0]})
        status, other_bot = request(second, base, '/v1/messages', {**shared, 'botId': 'bot-davin', 'lane': 'foreground'})
        check('nonce_scope_bot_isolated', status == 201 and other_bot['work']['requestId'] != ids[0], {'statusCode': status, 'otherBot': other_bot.get('work', {}).get('botId')})
        _, snapshot = request(first, base, '/v1/snapshot')
        _, mirror = request(second, base, '/v1/snapshot')
        check('two_local_clients_same_history', snapshot == mirror and len(snapshot['work']) == 3 and all(w['status'] == 'waiting_host' for w in snapshot['work']), {'workCount': len(snapshot['work']), 'equal': snapshot == mirror, 'runtime': snapshot['runtime']})
        cursor = snapshot['sequence']
        status, cancel = request(first, base, '/v1/cancel', {'requestId': ids[0]})
        req = urllib.request.Request(base + '/v1/events?after=0', headers={'Last-Event-ID': str(cursor)})
        with second.open(req, timeout=5) as response:
            first_line = response.readline().decode().strip()
        check('reconnect_cursor_returns_next_committed_event', first_line == 'id: ' + str(cursor + 1), {'cursor': cursor, 'firstLine': first_line})
        _, after_cancel = request(second, base, '/v1/snapshot')
        check('cancel_target_keeps_other_obligations', cancel['status'] == 'cancelled' and all(w['status'] == 'waiting_host' for w in after_cancel['work'] if w['requestId'] != ids[0]), {'statusCode': status, 'cancelStatus': cancel['status'], 'remaining': len(after_cancel['work']) - 1})
        process.kill()
        process.wait(timeout=10)
        process, base, command = start(directory)
        commands.append(command)
        first, second = client(base), client(base)
        _, restored = request(second, base, '/v1/snapshot')
        status, retried = request(first, base, '/v1/messages', shared)
        check('hard_kill_restart_acknowledged_identity', restored == after_cancel and status == 200 and retried['duplicate'] and retried['work']['requestId'] == ids[0], {'snapshotUnchanged': restored == after_cancel, 'statusCode': status, 'duplicate': retried.get('duplicate'), 'requestUnchanged': retried.get('work', {}).get('requestId') == ids[0]})
        status, _ = request(second, base, '/v1/events?after=' + str(restored['sequence'] + 100))
        check('ahead_cursor_rejects', status == 409, {'statusCode': status})
        process.kill()
        process.wait(timeout=10)
        process = None
except BaseException as error:
    checks.append({'id': 'execution', 'status': 'blocked', 'evidence': type(error).__name__ + ': ' + str(error)})
finally:
    if process is not None and process.poll() is None:
        process.kill()
        process.wait(timeout=10)
    report = {
        'codeCommit': args.commit,
        'binarySha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
        'command': [os.sys.executable, *os.sys.argv],
        'serverCommands': commands,
        'environment': {'platform': os.sys.platform, 'python': os.sys.version, 'listen': '127.0.0.1:0', 'state': 'owned TemporaryDirectory', 'providerCredentials': 'not inherited', 'host': 'not configured'},
        'checks': checks,
        'totals': {status: sum(c['status'] == status for c in checks) for status in ['pass', 'fail', 'blocked']},
        'durationSeconds': round(time.time() - started, 3),
        'scope': 'Two cookie jars sharing one local Go journal. No model, tool side effect, cloud sync, multi-host authentication, physical phone or mobile lifecycle acceptance.',
    }
    output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report, indent=2))
    raise SystemExit(2 if report['totals']['blocked'] else 1 if report['totals']['fail'] else 0)
