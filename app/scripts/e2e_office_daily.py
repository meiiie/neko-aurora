import argparse
import base64
import hashlib
import io
import json
import os
import pathlib
import queue
import subprocess
import threading
import urllib.error
import xml.sax.saxutils
import zipfile

from e2e_core import Client


def package(ext, text, external=False):
    escaped = xml.sax.saxutils.escape(text)
    if ext == 'xlsx':
        main = 'xl/workbook.xml'
        main_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml'
        part = 'xl/worksheets/sheet1.xml'
        part_type = 'application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml'
        members = {
            main: '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Quotes" sheetId="1" r:id="s1"/></sheets></workbook>',
            'xl/_rels/workbook.xml.rels': '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="s1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>',
            part: '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>' + escaped + '</t></is></c><c r="B1"><v>120</v></c><c r="C1"><f>SUM(B1)</f><v>120</v></c></row></sheetData></worksheet>',
        }
    else:
        main = 'ppt/presentation.xml'
        main_type = 'application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml'
        part = 'ppt/slides/slide1.xml'
        part_type = 'application/vnd.openxmlformats-officedocument.presentationml.slide+xml'
        members = {
            main: '<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="s1"/></p:sldIdLst></p:presentation>',
            'ppt/_rels/presentation.xml.rels': '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="s1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>',
            part: '<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>' + escaped + '</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>',
        }
    members['[Content_Types].xml'] = '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/' + main + '" ContentType="' + main_type + '"/><Override PartName="/' + part + '" ContentType="' + part_type + '"/></Types>'
    members['_rels/.rels'] = '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="' + main + '"/>' + ('<Relationship Id="link" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" TargetMode="External" Target="https://example.invalid/"/>' if external else '') + '</Relationships>'
    raw = io.BytesIO()
    with zipfile.ZipFile(raw, 'w', zipfile.ZIP_DEFLATED) as z:
        for name, content in sorted(members.items()):
            z.writestr(name, content)
    return raw.getvalue()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--go', type=pathlib.Path, required=True)
    parser.add_argument('--host', type=pathlib.Path, required=True)
    parser.add_argument('--go-tool', type=pathlib.Path, required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    args = parser.parse_args()
    out = args.output.resolve()
    assert out.as_posix().startswith('E:/CodexScratch/local-daily-use-20261012/')
    out.mkdir(parents=True, exist_ok=False)
    build = subprocess.check_output([str(args.go_tool), 'version', '-m', str(args.go)], text=True)
    assert 'vcs.revision=' + args.commit in build and 'vcs.modified=false' in build
    assert hashlib.sha256(args.host.read_bytes()).hexdigest() == '5beef30a02ac26768d26a90cdd9c4c1ed55ec99edd9687358d08a5ae22ae1423'
    report = {'testedApplication': args.commit, 'goSHA256': hashlib.sha256(args.go.read_bytes()).hexdigest(), 'hostSHA256': hashlib.sha256(args.host.read_bytes()).hexdigest(), 'checks': [], 'scope': 'Actual compiled Go authenticated HTTP upload, preview, original bytes, intake pins and restart; no Core/provider/GUI/phone/cloud.'}
    process = None
    stderr = (out / 'service-stderr.log').open('wb')
    env = {k: v for k, v in os.environ.items() if k.upper() in {'SYSTEMROOT', 'WINDIR', 'PATH', 'PATHEXT', 'COMSPEC', 'PROCESSOR_ARCHITECTURE', 'NUMBER_OF_PROCESSORS'}}
    env.update({'TEMP': str(out), 'TMP': str(out)})

    def check(name, value):
        report['checks'].append({'name': name, 'status': 'PASS' if value else 'FAIL'})
        assert value, name

    def launch():
        nonlocal process
        process = subprocess.Popen([str(args.go), '--listen', '127.0.0.1:0', '--state', str(out / 'private-state'), '--host-executable', str(args.host)], stdout=subprocess.PIPE, stderr=stderr, stdin=subprocess.DEVNULL, text=True, env=env, creationflags=subprocess.CREATE_NO_WINDOW)
        ready = queue.Queue()
        threading.Thread(target=lambda: ready.put(process.stdout.readline().strip()), daemon=True).start()
        line = ready.get(timeout=15)
        assert line.startswith('READY http://127.0.0.1:'), 'fixture_startup'
        return Client(line[6:])

    def stop():
        if process and process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
        if process and process.stdout:
            process.stdout.close()

    def denied(client, path, body=None, expected=409):
        try:
            client.request(path, body)
        except urllib.error.HTTPError as error:
            return error.code == expected
        return False

    try:
        client = launch()
        check('office_capability_advertised', 'local_office_text_v1' in client.request('/v1/snapshot')['capabilities'])
        channel = client.request('/v1/channels', {'name': 'Office quote review', 'botIds': ['bot-meiiie', 'bot-davin'], 'clientNonce': 'office-http-channel'})['channel']
        project = client.request('/v1/projects', {'name': 'Office quotes', 'goal': 'Inspect literal quotes', 'channelIds': [channel['id']], 'clientNonce': 'office-http-project'})['project']
        accepted = []
        for ext in ['xlsx', 'pptx']:
            text = 'Báo giá tiếng Việt: 120 dollars'
            raw = package(ext, text)
            body = {'projectId': project['id'], 'projectRevision': 1, 'channelId': channel['id'], 'filename': 'quote.' + ext, 'clientNonce': 'office-http-upload-' + ext, 'contentBase64': base64.b64encode(raw).decode()}
            file = client.request('/v1/uploads', body)['file']
            check(ext + '_ready_with_original_and_text_provenance', file['status'] == 'ready_text' and file['extractionVersion'] == 1 and file['sha256'] == hashlib.sha256(raw).hexdigest() and file['textBytes'] > 0 and len(file['textSHA256']) == 64 and not file.get('excerpt'))
            path = '/v1/projects/' + project['id'] + '/files/' + file['id']
            preview_path = path + '/text?channelId=' + channel['id'] + '&projectRevision=1&accessRevision=0'
            seq = client.request('/v1/snapshot')['sequence']
            preview = client.request(preview_path)
            check(ext + '_literal_unicode_preview_no_mutation', text in preview['text'] and preview['textSHA256'] == file['textSHA256'] and '.xml' in preview['text'] and client.request('/v1/snapshot')['sequence'] == seq)
            if ext == 'xlsx':
                check('xlsx_cached_value_not_formula_source', 'SUM(B1)' not in preview['text'] and '120' in preview['text'])
            content, headers = client.request(path + '?channelId=' + channel['id'])
            check(ext + '_download_original_bytes_hash', content == raw and headers['X-File-SHA256'] == file['sha256'])
            work = client.request('/v1/messages', {'botId': 'bot-meiiie', 'text': 'Inspect quote', 'lane': 'background', 'clientNonce': 'office-http-task-' + ext, 'channelId': channel['id'], 'expectedChannelRevision': 1, 'projectId': project['id'], 'projectRevision': 1, 'fileIds': [file['id']]})['work']
            check(ext + '_request_pins_extracted_text_and_original_hash', len(work['resources']['files']) == 1 and text in work['resources']['files'][0]['excerpt'] and work['resources']['files'][0]['sha256'] == file['sha256'])
            client.request('/v1/file-access', {'fileId': file['id'], 'expectedRevision': 0, 'granted': False, 'clientNonce': 'office-http-revoke-' + ext})
            check(ext + '_revoked_preview_denied', denied(client, preview_path))
            accepted.append((body, file, preview_path))
            bad = dict(body, filename='external.' + ext, clientNonce='office-http-external-' + ext, contentBase64=base64.b64encode(package(ext, text, True)).decode())
            quarantine = client.request('/v1/uploads', bad)['file']
            check(ext + '_external_relationship_quarantined', quarantine['status'] == 'quarantined_unsupported')
            sensitive = 'ordinary ' * 2000 + 'api_' + 'key="' + 'x' * 32 + '"'
            before = client.request('/v1/snapshot')['sequence']
            bad = dict(body, filename='sensitive.' + ext, clientNonce='office-http-sensitive-' + ext, contentBase64=base64.b64encode(package(ext, sensitive)).decode())
            check(ext + '_full_extracted_screening_before_commit', denied(client, '/v1/uploads', bad, 400) and client.request('/v1/snapshot')['sequence'] == before)
        pdf = client.request('/v1/uploads', {'projectId': project['id'], 'projectRevision': 1, 'channelId': channel['id'], 'filename': 'unsupported.pdf', 'clientNonce': 'office-http-pdf', 'contentBase64': base64.b64encode(b'%PDF-1.7 synthetic unsupported').decode()})['file']
        check('pdf_remains_quarantined', pdf['status'] == 'quarantined_unsupported')
        for index in range(3):
            stop()
            client = launch()
            for body, file, preview_path in accepted:
                duplicate = client.request('/v1/uploads', body)
                check('restart_' + str(index + 1) + '_' + body['filename'] + '_original_receipt_does_not_regrant', duplicate['duplicate'] and duplicate['file']['id'] == file['id'] and not duplicate['file']['granted'] and denied(client, preview_path))
        report['status'] = 'PASS'
    except BaseException as error:
        report['status'] = 'FAIL'
        report['failure'] = type(error).__name__ + ': ' + str(error)[:200]
        raise
    finally:
        stop()
        stderr.close()
        report['fixtureExited'] = process is None or process.poll() is not None
        (out / 'validation.json').write_text(json.dumps(report, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
        print(json.dumps({'status': report['status'], 'checks': len(report['checks']), 'fixtureExited': report['fixtureExited']}))


if __name__ == '__main__':
    main()
