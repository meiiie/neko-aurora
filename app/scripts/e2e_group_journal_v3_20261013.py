"""Real HTTP journal V1/V2/V3 compatibility; pinned binaries and private local state."""
import argparse
import hashlib
import http.cookiejar
import json
import os
import pathlib
import queue
import shutil
import subprocess
import threading
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
SCRATCH = pathlib.Path('E:/CodexScratch/group-journal-binary-20261013').resolve()
GO = pathlib.Path('C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe')
LEGACY = {
    1: (pathlib.Path('E:/CodexScratch/desktop-instance-20261012/candidate-c91e4de/.build/desktop-c91e4de/neko-control.exe'), 'a7663f40fb0d3893f64e1fac8f933d42672812228b6ffb7837665d01bb6ecd03'),
    2: (pathlib.Path('E:/CodexScratch/parallel-product-20261013/candidate-5349523/.build/release/neko-control.exe'), 'c2732194124fb989dc84b301fb06e910f64579e94f79e930bf57b1b91ffa337d'),
}


def sha(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=pathlib.Path, required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--code', required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    args = parser.parse_args()
    out = args.output.resolve()
    assert out.is_relative_to(SCRATCH) and out != SCRATCH
    assert len(args.code) == 40 and len(args.sha256) == 64
    out.mkdir(parents=True, exist_ok=False)
    private = out / 'private-fixture'
    private.mkdir()
    env = {key: os.environ[key] for key in ['SYSTEMROOT', 'WINDIR', 'PATH', 'PATHEXT', 'COMSPEC', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'] if key in os.environ}
    env.update({'TEMP': str(private), 'TMP': str(private), 'USERPROFILE': str(private)})
    result = {'status': 'RUNNING', 'code': args.code, 'runnerSHA256': sha(__file__), 'command': [str(pathlib.Path(__file__).resolve()), '--binary', str(args.binary), '--sha256', args.sha256, '--code', args.code, '--output', str(out)], 'environment': {'platform': 'Windows amd64', 'python': '3.13', 'namedEnvKeys': sorted(env)}, 'binarySHA256': sha(args.binary), 'checks': [], 'launches': [], 'scope': 'Compiled local Go only. Fresh journals created through HTTP; no source journal rewriting, Core/host/model/provider/browser/phone/cloud or live state.'}
    processes, logs = [], []

    def check(name, condition):
        result['checks'].append({'name': name, 'status': 'PASS' if condition else 'FAIL'})
        assert condition, name

    def stop(process):
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=10)

    def launch(binary, state):
        path = private / ('service-' + str(len(processes)) + '.stderr')
        log = path.open('x', encoding='utf-8')
        logs.append(log)
        command = [str(binary.resolve()), '--listen', '127.0.0.1:0', '--state', str(state)]
        process = subprocess.Popen(command, cwd=ROOT, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=log, text=True, encoding='utf-8')
        processes.append(process)
        lines = queue.Queue()
        threading.Thread(target=lambda: lines.put(process.stdout.readline().strip()), daemon=True).start()
        line = lines.get(timeout=15)
        result['launches'].append({'binarySHA256': sha(binary), 'command': command, 'pid': process.pid, 'startupLine': line})
        return process, line, path

    def client(endpoint):
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        opener.open(endpoint, timeout=5).close()

        def call(route, body=None):
            request = urllib.request.Request(endpoint + route, data=json.dumps(body).encode() if body is not None else None, headers={'Origin': endpoint, 'Content-Type': 'application/json'})
            try:
                with opener.open(request, timeout=5) as response:
                    return response.status, json.loads(response.read())
            except urllib.error.HTTPError as error:
                return error.code, json.loads(error.read())
        return call

    def create(call, version):
        nonce = 'journal-v3-compat-policy-' + str(version)
        code, value = call('/v1/channels', {'name': 'Private compatibility fixture', 'botIds': ['bot-meiiie', 'bot-davin'], 'expectedRevision': 0, 'clientNonce': nonce + '-channel'})
        assert code == 200
        channel = value['channel']
        body = {'channelId': channel['id'], 'expectedChannelRevision': channel['revision'], 'participants': ['bot-meiiie', 'bot-davin'], 'coordinatorBotId': 'bot-meiiie', 'goal': 'Prove exact frozen local journal policy without execution', 'maxStages': 3, 'clientNonce': nonce}
        code, value = call('/v1/collaborations', body)
        assert code == 201
        return value['collaboration'], body

    def frozen(state, group_id):
        for line in (state / 'transactions.jsonl').read_text(encoding='utf-8').splitlines():
            event = json.loads(line)['event']
            group = event.get('collaboration')
            if event['type'] == 'collaboration.created' and group['id'] == group_id:
                return group['publishedReportContext']
        raise AssertionError('Missing genuine group creation event')

    try:
        check('new_binary_matches_supplied_hash', sha(args.binary) == args.sha256)
        info = subprocess.check_output([str(GO), 'version', '-m', str(args.binary)], env=env, text=True, encoding='utf-8')
        (out / 'build-info.txt').write_text(info, encoding='utf-8')
        check('new_binary_embeds_exact_clean_code', 'vcs.revision=' + args.code in info and 'vcs.modified=false' in info)
        state = private / 'new-v3'
        process, line, _ = launch(args.binary, state)
        check('new_binary_starts_with_fresh_state', line.startswith('READY http://127.0.0.1:'))
        group, body = create(client(line[6:]), 3)
        stop(process)
        pin = frozen(state, group['id'])
        check('real_http_created_policy_v3', pin['version'] == 3)
        process, line, _ = launch(args.binary, state)
        check('new_binary_restarts_policy_v3', line.startswith('READY http://127.0.0.1:'))
        code, reply = client(line[6:])('/v1/collaborations', body)
        check('v3_nonce_and_identity_survive_restart', code == 200 and reply['duplicate'] and reply['collaboration']['id'] == group['id'])
        stop(process)
        check('v3_original_policy_pin_unchanged', frozen(state, group['id']) == pin)
        for version, (binary, digest) in LEGACY.items():
            check('legacy_v' + str(version) + '_binary_hash', sha(binary) == digest)
            rejected = private / ('v' + str(version) + '-rejects-v3')
            shutil.copytree(state, rejected)
            before = sha(rejected / 'transactions.jsonl')
            process, line, stderr = launch(binary, rejected)
            process.wait(timeout=10)
            logs[-1].flush()
            check('v' + str(version) + '_rejects_v3_before_ready', not line.startswith('READY ') and process.returncode != 0 and 'Local store unavailable; inspect state before retry' in line + stderr.read_text(encoding='utf-8'))
            check('v' + str(version) + '_rejected_copy_is_unchanged', sha(rejected / 'transactions.jsonl') == before)
            legacy_state = private / ('genuine-v' + str(version))
            process, line, _ = launch(binary, legacy_state)
            check('legacy_v' + str(version) + '_fresh_state_opens', line.startswith('READY http://127.0.0.1:'))
            old_group, old_body = create(client(line[6:]), version)
            stop(process)
            old_pin = frozen(legacy_state, old_group['id'])
            check('legacy_http_created_v' + str(version), old_pin['version'] == version)
            process, line, _ = launch(args.binary, legacy_state)
            check('new_reader_opens_genuine_v' + str(version), line.startswith('READY http://127.0.0.1:'))
            code, reply = client(line[6:])('/v1/collaborations', old_body)
            check('legacy_v' + str(version) + '_duplicate_identity_retained', code == 200 and reply['duplicate'] and reply['collaboration']['id'] == old_group['id'])
            stop(process)
            check('upgrade_preserves_frozen_v' + str(version), frozen(legacy_state, old_group['id']) == old_pin)
        result['status'] = 'PASS'
    except Exception as error:
        result.update({'status': 'FAIL', 'errorType': type(error).__name__, 'error': str(error)})
    finally:
        for process in processes:
            stop(process)
        for log in logs:
            log.close()
        result['ownedProcessesStopped'] = all(process.poll() is not None for process in processes)
        (out / 'results.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({key: result[key] for key in ['status', 'checks', 'ownedProcessesStopped']}))
    return int(result['status'] != 'PASS')


if __name__ == '__main__':
    raise SystemExit(main())
