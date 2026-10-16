import argparse
import hashlib
import io
import json
import os
import pathlib
import queue
import struct
import subprocess
import sys
import threading
import urllib.error
import zipfile


def document(text):
    from xml.sax.saxutils import escape
    parts = {
        '[Content_Types].xml': '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>',
        '_rels/.rels': '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>',
        'word/document.xml': '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>' + escape(text) + '</w:t></w:r></w:p></w:body></w:document>',
    }
    raw = io.BytesIO()
    with zipfile.ZipFile(raw, 'w', zipfile.ZIP_DEFLATED) as archive:
        for name in sorted(parts):
            archive.writestr(name, parts[name])
    return raw.getvalue()


def request_frame(format_id, raw):
    return b'NDW1' + bytes([format_id]) + struct.pack('>I', len(raw)) + raw


def response_frame(raw):
    if len(raw) < 41 or raw[:4] != b'NDR1':
        return None
    count = struct.unpack('>I', raw[5:9])[0]
    if len(raw) != 41 + count or count > 512 * 1024:
        return None
    text = raw[41:]
    if raw[4] == 0:
        return {'ok': False} if count == 0 and raw[9:41] == bytes(32) else None
    if raw[4] != 1 or raw[9:41] != hashlib.sha256(text).digest():
        return None
    try:
        return {'ok': True, 'text': text.decode('utf-8'), 'sha256': raw[9:41].hex()}
    except UnicodeDecodeError:
        return None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=pathlib.Path, required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--code', required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    parser.add_argument('--baseline', action='store_true')
    parser.add_argument('--immutable', action='store_true')
    args = parser.parse_args()
    if hashlib.sha256(args.binary.read_bytes()).hexdigest() != args.sha256.lower():
        raise SystemExit('binary_hash_mismatch')
    source_root = pathlib.Path(__file__).resolve().parents[1]
    if args.immutable:
        if args.baseline:
            raise SystemExit('immutable_baseline_conflict')
        gate_env = {name: os.environ[name] for name in ['SystemRoot', 'WINDIR', 'COMSPEC'] if name in os.environ}
        gate_env.update(USERPROFILE=str(args.output / 'profile'), HOME=str(args.output / 'profile'), GOENV='off', GOWORK='off', GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', CGO_ENABLED='0')
        git = 'C:/Program Files/Git/cmd/git.exe'
        head = subprocess.check_output([git, '--no-optional-locks', '-C', str(source_root), 'rev-parse', 'HEAD'], env=gate_env, text=True, stderr=subprocess.DEVNULL).strip()
        dirty = subprocess.check_output([git, '--no-optional-locks', '-C', str(source_root), 'status', '--porcelain', '--untracked-files=all'], env=gate_env, stderr=subprocess.DEVNULL)
        if head != args.code or dirty:
            raise SystemExit('immutable_source_not_exact_clean')
        go = 'C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe'
        metadata = subprocess.check_output([go, 'version', '-m', str(args.binary)], env=gate_env, text=True, stderr=subprocess.DEVNULL)
        if 'vcs.revision=' + args.code not in metadata or 'vcs.modified=false' not in metadata:
            raise SystemExit('immutable_binary_not_exact_clean')
    args.output.mkdir(parents=True, exist_ok=False)
    for name in ['temp', 'profile']:
        (args.output / name).mkdir()
    env = {name: os.environ[name] for name in ['SystemRoot', 'WINDIR', 'COMSPEC'] if name in os.environ}
    env.update(TEMP=str(args.output / 'temp'), TMP=str(args.output / 'temp'), USERPROFILE=str(args.output / 'profile'))
    source_paths = ['native/cmd/neko-control/main.go', 'native/internal/ledger/file_text.go', 'native/internal/ledger/projects.go', 'native/internal/ledger/store.go', 'native/internal/ledger/document_worker_test.go', 'native/internal/documentworker/worker.go', 'native/internal/documentworker/worker_test.go', 'native/internal/documentworker/process_windows.go', 'native/internal/documentworker/process_other.go', 'native/internal/documents/docx.go', 'native/internal/documents/office_text.go']
    report = {'code': args.code, 'workingTree': not (args.baseline or args.immutable), 'binarySHA256': args.sha256.lower(), 'command': sys.argv, 'environment': {'platform': sys.platform, 'python': sys.version.split()[0], 'childVariableNames': sorted(env), 'privateRoot': str(args.output)}, 'scope': 'Genuine local worker and service upload/preview/replay; ledger tool preparation is separate, no Core/model/renderer/phone acceptance', 'sourceSHA256': {name: hashlib.sha256((source_root / name).read_bytes()).hexdigest() for name in source_paths if (source_root / name).exists()}, 'checks': []}
    process = None

    def check(name, value):
        report['checks'].append({'name': name, 'status': 'PASS' if value else 'FAIL'})

    def run(frame, extra=None):
        process = subprocess.run([str(args.binary), '--document-worker-v1'] + (extra or []), input=frame, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20, cwd=args.output, env=env, creationflags=subprocess.CREATE_NO_WINDOW)
        return process.returncode, response_frame(process.stdout), len(process.stdout), len(process.stderr)

    def launch_service():
        nonlocal process
        from e2e_core import Client
        process = subprocess.Popen([str(args.binary), '--listen', '127.0.0.1:0', '--state', str(args.output / 'service-state')], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, cwd=args.output, env=env, creationflags=subprocess.CREATE_NO_WINDOW)
        ready = queue.Queue()
        threading.Thread(target=lambda: ready.put(process.stdout.readline().strip()), daemon=True).start()
        line = ready.get(timeout=15)
        if not line.startswith('READY http://127.0.0.1:'):
            raise AssertionError('service_startup')
        return Client(line[6:])

    def stop_service():
        nonlocal process
        if process is not None:
            if process.poll() is None:
                process.terminate()
                process.wait(timeout=10)
            process.stdout.close()
            process = None

    def service_checks():
        from e2e_office_daily import package
        import base64
        client = launch_service()
        channel = client.request('/v1/channels', {'name': 'Worker source review', 'botIds': ['bot-meiiie', 'bot-davin'], 'clientNonce': 'worker-service-channel'})['channel']
        project = client.request('/v1/projects', {'name': 'Worker source fixture', 'goal': 'Verify ordinary literal text', 'channelIds': [channel['id']], 'clientNonce': 'worker-service-project'})['project']
        original = []
        for ext, format_id in [('docx', 1), ('xlsx', 2), ('pptx', 3)]:
            text = 'Local worker tiếng Việt ' + ext
            raw = document(text) if ext == 'docx' else package(ext, text)
            code, expected, _, _ = run(request_frame(format_id, raw))
            check(ext + '_genuine_worker_positive', code == 0 and expected and expected['ok'] and text in expected['text'])
            body = {'projectId': project['id'], 'projectRevision': 1, 'channelId': channel['id'], 'filename': 'source.' + ext, 'clientNonce': 'worker-service-upload-' + ext, 'contentBase64': base64.b64encode(raw).decode()}
            file = client.request('/v1/uploads', body)['file']
            check(ext + '_actual_upload_exact_text_and_raw_identity', file['status'] == 'ready_text' and file['textSHA256'] == expected['sha256'] and file['sha256'] == hashlib.sha256(raw).hexdigest())
            route = '/v1/projects/' + project['id'] + '/files/' + file['id']
            preview_route = route + '/text?channelId=' + channel['id'] + '&projectRevision=1&accessRevision=0'
            before = client.request('/v1/snapshot')['sequence']
            preview = client.request(preview_route)
            check(ext + '_actual_preview_reparse_identity_without_journal_write', preview['text'] == expected['text'] and preview['textSHA256'] == file['textSHA256'] and before == client.request('/v1/snapshot')['sequence'])
            content, headers = client.request(route + '?channelId=' + channel['id'])
            check(ext + '_actual_original_download', content == raw and headers['X-File-SHA256'] == file['sha256'])
            duplicate = client.request('/v1/uploads', body)
            check(ext + '_actual_nonce_receipt_preserved', duplicate['duplicate'] and duplicate['file']['id'] == file['id'])
            client.request('/v1/file-access', {'fileId': file['id'], 'expectedRevision': 0, 'granted': False, 'clientNonce': 'worker-service-revoke-' + ext})
            try:
                client.request(preview_route)
                denied = False
            except urllib.error.HTTPError as error:
                denied = error.code == 409
            check(ext + '_actual_revoked_preview_denied', denied)
            original.append((body, file))
        raw = document('Large valid text ' + 'q' * 400000)
        code, large, _, _ = run(request_frame(1, raw))
        check('large_valid_fixture_within_deadline', code == 0 and large and large['ok'] and len(large['text'].encode('utf-8')) > 400000)
        bad = dict(original[0][0], filename='invalid.docx', clientNonce='worker-service-invalid', contentBase64=base64.b64encode(b'not an archive').decode())
        quarantine = client.request('/v1/uploads', bad)['file']
        check('actual_invalid_parser_output_quarantined', quarantine['status'] == 'quarantined_unsupported' and not quarantine.get('textSHA256'))
        stop_service()
        client = launch_service()
        for body, file in original:
            duplicate = client.request('/v1/uploads', body)
            check(body['filename'] + '_actual_restart_dedup_no_regrant', duplicate['duplicate'] and duplicate['file']['id'] == file['id'] and not duplicate['file']['granted'])
        check('service_restart_keeps_quarantine_receipt', client.request('/v1/uploads', bad)['file']['id'] == quarantine['id'])
        stop_service()

    try:
        text = 'Bounded local document: tiếng Việt 120 dollars'
        raw = document(text)
        code, response, _, _ = run(request_frame(1, raw))
        check('genuine_worker_docx_exact_result', code == 0 and response and response['ok'] and text in response['text'])
        check('worker_has_no_state_or_listener_side_effect', not (args.output / '.state').exists())
        if not args.baseline:
            for name, frame in [('bad_magic', b'BAD1' + request_frame(1, raw)[4:]), ('bad_format', request_frame(9, raw)), ('trailing_frame', request_frame(1, raw) + request_frame(1, raw)), ('partial_input', request_frame(1, raw)[:-1]), ('over_raw_quota', b'NDW1\x01' + struct.pack('>I', (1 << 20) + 1)), ('unsafe_document', request_frame(1, b'not a zip'))]:
                code, response, _, _ = run(frame)
                check(name + '_rejected_without_text', response is not None and not response['ok'])
            code, _, _, _ = run(request_frame(1, raw), ['--state', str(args.output / 'forbidden-state')])
            check('worker_mode_rejects_normal_flags_before_state', code != 0 and not (args.output / 'forbidden-state').exists())
            service_checks()
        report['status'] = 'PASS' if all(case['status'] == 'PASS' for case in report['checks']) else 'FAIL'
    except BaseException as error:
        report['status'] = 'FAIL'
        report['failureType'] = type(error).__name__
    finally:
        stop_service()
        report['ownedServiceExited'] = process is None
        (args.output / 'results.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'status': report['status'], 'checks': len(report['checks'])}))
    return 0 if report['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
