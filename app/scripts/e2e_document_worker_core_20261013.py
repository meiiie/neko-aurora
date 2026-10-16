"""Pinned Go/Rust/Core tool-read composition on authored Office files and loopback only."""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import queue
import subprocess
import sys
import threading
import urllib.error


def main():
    parser = argparse.ArgumentParser()
    for name in ['source', 'binary', 'sha256', 'code', 'output']:
        parser.add_argument('--' + name, required=True)
    args = parser.parse_args()
    source = pathlib.Path(args.source).resolve()
    binary = pathlib.Path(args.binary).resolve()
    out = pathlib.Path(args.output).resolve()
    assert source == pathlib.Path('E:/CodexScratch/office-worker-20261013/combined-source').resolve()
    assert pathlib.Path('E:/CodexScratch/office-worker-20261013').resolve() in out.parents
    sha = lambda p: hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
    assert sha(binary) == args.sha256
    git = 'C:/Program Files/Git/cmd/git.exe'
    assert subprocess.check_output([git, 'rev-parse', 'HEAD'], cwd=source, text=True).strip() == args.code
    assert not subprocess.check_output([git, 'status', '--porcelain'], cwd=source)
    candidate = pathlib.Path('E:/CodexScratch/continued-product-20261013/candidate-5d405b7/.build/release')
    pins = {
        'host': (candidate / 'neko-host.exe', 'a1693c4f3309c86e29b348ffc913c888cae3f8fd2bdf04bcac418e107a6654d2'),
        'core': (candidate / 'neko-core.exe', '356247616082e613abd99b97838fd533f5e14ab64b07f480d78ae799d10648bf'),
        'writer': (candidate / 'neko-writer-lease.exe', 'f9f404a07a7fe1aa56a2b7d3c073a2330366de5ee26d572b37421338e2b99a3a'),
    }
    assert all(sha(path) == digest for path, digest in pins.values())
    sys.path.insert(0, str(source / 'scripts'))
    from e2e_core import Client
    from e2e_document_worker_20261013 import document
    from e2e_office_daily import package
    out.mkdir(exist_ok=False)
    home = out / 'home'
    home.mkdir()
    env = {key: os.environ[key] for key in ['SystemRoot', 'WINDIR', 'COMSPEC'] if key in os.environ}
    env.update(USERPROFILE=str(home), HOME=str(home), TEMP=str(home), TMP=str(home), APPDATA=str(home / 'roaming'), LOCALAPPDATA=str(home / 'local'))
    checks, ids, processes = [], [], []
    client = None
    def check(name, value):
        checks.append({'name': name, 'status': 'PASS' if value else 'FAIL'})
        assert value, name
    def ready(process):
        lines = queue.Queue()
        threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True).start()
        line = lines.get(timeout=20).strip()
        assert line.startswith('READY http://127.0.0.1:'), 'owned_process_not_ready'
        return line[6:]
    logs = []
    report = {'status': 'FAIL', 'codeCommit': args.code, 'binarySHA256': args.sha256, 'runnerSHA256': sha(__file__), 'command': sys.argv, 'environmentNames': sorted(env), 'checks': checks, 'pins': {name: {'path': str(path), 'sha256': digest} for name, (path, digest) in pins.items()}, 'scope': 'Actual compiled Go mandatory self-worker, pinned Rust host and Windows Core MCP project.read_file on synthetic Office files, loopback fixture responses. No real credential, model quality, GUI/phone/cloud, power-loss or hard OS sandbox acceptance.'}
    try:
        fixture_log = (out / 'fixture-stderr.txt').open('xb'); logs.append(fixture_log)
        fixture = subprocess.Popen([sys.executable, str(source / 'scripts/fixture_provider.py')], cwd=out, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=fixture_log, text=True, creationflags=subprocess.CREATE_NO_WINDOW)
        processes.append(fixture)
        endpoint = ready(fixture)
        config = out / 'fixture.json'
        config.write_text(json.dumps({'hostExecutable': str(pins['host'][0]), 'coreExecutable': str(pins['core'][0]), 'supervisor': str(source / 'scripts/core_supervisor.py'), 'fixtureEndpoint': endpoint}), encoding='utf-8')
        service_log = (out / 'service-stderr.txt').open('xb'); logs.append(service_log)
        service = subprocess.Popen([str(binary), '--state', str(out / 'state'), '--listen', '127.0.0.1:0', '--core-fixture-config', str(config)], cwd=out, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=service_log, text=True, creationflags=subprocess.CREATE_NO_WINDOW)
        processes.append(service)
        client = Client(ready(service))
        channel = client.request('/v1/channels', {'name': 'Document worker Core fixture', 'botIds': ['bot-meiiie'], 'clientNonce': 'worker-core-channel'})['channel']
        check('owned_runtime_channel_ready', channel['revision'] == 1)
        works = []
        for ext in ['docx', 'xlsx', 'pptx']:
            project = client.request('/v1/projects', {'name': 'Worker ' + ext, 'channelIds': [channel['id']], 'clientNonce': 'worker-core-project-' + ext})['project']
            literal = 'Worker Core tiếng Việt ' + ext + ': price 120 dollars; literal source only.'
            raw = document(literal) if ext == 'docx' else package(ext, literal)
            file = client.request('/v1/uploads', {'projectId': project['id'], 'projectRevision': 1, 'channelId': channel['id'], 'filename': 'source.' + ext, 'contentBase64': base64.b64encode(raw).decode(), 'clientNonce': 'worker-core-upload-' + ext})['file']
            check(ext + '_mandatory_worker_ready_with_original_hash', file['status'] == 'ready_text' and file['sha256'] == hashlib.sha256(raw).hexdigest() and file['textBytes'] > 0)
            body = {'botId': 'bot-meiiie', 'channelId': channel['id'], 'expectedChannelRevision': 1, 'projectId': project['id'], 'projectRevision': 1, 'fileIds': [file['id']], 'lane': 'foreground', 'text': '[fixture:file-read] Read this literal Office source.', 'clientNonce': 'worker-core-intake-' + ext}
            work = client.request('/v1/messages', body)['work']; ids.append(work['requestId'])
            done = client.wait(work['requestId'], lambda w: w['status'] == 'completed' and w.get('runtimeStopped'), timeout=45)
            reply = done.get('reply', '')
            pin = done.get('resources', {}).get('files', [])
            assert reply.startswith('READ_RESULT='), 'missing_typed_tool_result'
            result = json.loads(reply[len('READ_RESULT='):])
            report.setdefault('readObservations', []).append({'extension': ext, 'toolTextSHA256': result.get('sha256'), 'originalSHA256': file['sha256'], 'extractedTextSHA256': file['textSHA256'], 'literalMatched': literal in result.get('text', ''), 'eof': result.get('eof')})
            check(ext + '_genuine_Core_MCP_reads_reparsed_text', literal in result.get('text', '') and result.get('sha256') == file['textSHA256'] and hashlib.sha256(result['text'].encode('utf-8')).hexdigest() == file['textSHA256'] and result.get('eof') is True and len(pin) == 1 and pin[0]['id'] == file['id'] and pin[0]['sha256'] == file['sha256'] and pin[0]['textSHA256'] == file['textSHA256'] and done['core']['contextAdmission']['manifestSHA256'] == done['runtimeContextSHA256'])
            duplicate = client.request('/v1/messages', body)
            check(ext + '_original_intake_receipt_without_second_execution', duplicate['duplicate'] and duplicate['work']['requestId'] == work['requestId'] and duplicate['work']['core']['sessionId'] == done['core']['sessionId'])
            works.append({'extension': ext, 'requestId': work['requestId'], 'fileId': file['id'], 'originalSHA256': file['sha256'], 'textSHA256': file['textSHA256'], 'sessionId': done['core']['sessionId'], 'runtimeStopReceipt': done['runtimeStopReceipt']})
        report['works'] = works
        report['status'] = 'PASS'
    except BaseException as error:
        report['failure'] = type(error).__name__ + ': ' + str(error)[:500]
    finally:
        if client is not None:
            for identity in ids:
                try:
                    work = client.work(identity)
                    if not work.get('runtimeStopped'):
                        client.request('/v1/cancel', {'requestId': identity})
                        client.wait(identity, lambda w: w.get('runtimeStopped'), timeout=20)
                except BaseException as error:
                    report.setdefault('cleanupFailures', []).append(type(error).__name__)
        for process in reversed(processes):
            if process.stdin:
                process.stdin.close()
            if process.poll() is None:
                if process is processes[0]:
                    try: process.wait(timeout=5)
                    except subprocess.TimeoutExpired: process.terminate(); process.wait(timeout=5)
                else:
                    process.terminate(); process.wait(timeout=5)
            if process.stdout: process.stdout.close()
        for log in logs: log.close()
        report['ownedProcessesExited'] = all(process.poll() is not None for process in processes)
        report['pinsStillMatch'] = all(sha(path) == digest for path, digest in pins.values()) and sha(binary) == args.sha256
        if report.get('cleanupFailures') or not report['pinsStillMatch']: report['status'] = 'FAIL'
        (out / 'results.json').write_text(json.dumps(report, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
        print(json.dumps({'status': report['status'], 'checks': len(checks), 'failure': report.get('failure'), 'ownedProcessesExited': report['ownedProcessesExited'], 'out': str(out)}))
    raise SystemExit(report['status'] != 'PASS')


if __name__ == '__main__':
    main()
