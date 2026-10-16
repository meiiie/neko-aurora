"""Actual Go/Rust/Core published-report follow-up acceptance with a loopback fixture."""
import argparse
import hashlib
import json
import os
import pathlib
import platform
import queue
import subprocess
import sys
import threading
import time
import urllib.error

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'scripts'))
from e2e_core import Client
from e2e_tag_collaboration_daily import CoreHandle

HOST_HASH = '610735eb4aea161ae448096b88292051d491a32638d170655d8459f2805dcc1a'
CORE_HASH = '45d6191adc9dc4b66ee46b363676eb1706c78c01818946b570087037aaf94e52'


def sha(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def ready(process):
    q = queue.Queue()
    threading.Thread(target=lambda: q.put(process.stdout.readline().strip()), daemon=True).start()
    line = q.get(timeout=20)
    assert line.startswith('READY http://127.0.0.1:')
    return line[6:]


def read_result(value):
    if isinstance(value, str):
        try:
            return read_result(json.loads(value))
        except ValueError:
            return None
    if isinstance(value, dict):
        if set(value) == {'fileId', 'sha256', 'offset', 'nextOffset', 'eof', 'text', 'authority'}:
            return value
        value = list(value.values())
    if isinstance(value, list):
        for item in value:
            found = read_result(item)
            if found:
                return found
    return None


def persisted_core_calls(state, work):
    path = state / 'tasks' / work['requestId'] / '.core-home/.neko-core/task-sessions' / (work['core']['sessionId'] + '.json')
    value = json.loads(path.read_text(encoding='utf-8'))
    def objects(item):
        if isinstance(item, dict):
            yield item
            for child in item.values():
                yield from objects(child)
        elif isinstance(item, list):
            for child in item:
                yield from objects(child)
    return [call for item in objects(value.get('tasks', [])) if item.get('role') == 'assistant' for call in item.get('tool_calls', [])]


def main():
    parser = argparse.ArgumentParser()
    for flag in ['go', 'host', 'core', 'go-tool', 'fixture', 'output']:
        parser.add_argument('--' + flag, required=True, type=pathlib.Path)
    parser.add_argument('--commit', required=True)
    args = parser.parse_args()
    out = args.output.resolve()
    assert out.is_relative_to(pathlib.Path('E:/CodexScratch/report-core-context-20261012').resolve())
    out.mkdir(parents=True, exist_ok=False)
    private = out / 'private-fixture'
    private.mkdir()
    checks = []
    report = {'status': 'RUNNING', 'applicationCommit': args.commit, 'sourceHeadAtRun': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip(), 'sourceWorkingTreeAtRun': bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT)), 'runnerSHA256': sha(__file__), 'fixtureSHA256': sha(args.fixture), 'command': [sys.executable, *sys.argv], 'environment': {'python': platform.python_version(), 'platform': platform.platform()}, 'scope': 'Genuine Windows Go/Rust/Core and controlled loopback provider. Original private report and selected/unselected output are generated through actual Core tool calls. This is local protocol/context/effect acceptance, not model quality, production provider, multi-human transport, cloud, browser/Tauri/phone/AT.', 'checks': checks}
    def check(name, condition):
        checks.append({'name': name, 'status': 'PASS' if condition else 'FAIL'})
        assert condition, name
    environment = {k: v for k, v in os.environ.items() if k.upper() in {'SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'}}
    environment.update({'TEMP': str(private), 'TMP': str(private), 'PYTHONDONTWRITEBYTECODE': '1'})
    fixture = service = None
    logs, handles = [], []
    failure = None
    try:
        info = subprocess.check_output([str(args.go_tool), 'version', '-m', str(args.go)]).decode()
        report['buildInfo'] = info
        report['binaryHashes'] = {n: sha(p) for n, p in [('go',args.go),('host',args.host),('core',args.core)]}
        check('exact_clean_Go_and_reviewed_Rust_Core_binary_pins', report['sourceHeadAtRun'] == args.commit and not report['sourceWorkingTreeAtRun'] and 'vcs.revision=' + args.commit in info and 'vcs.modified=false' in info and sha(args.host) == HOST_HASH and sha(args.core) == CORE_HASH)
        logs.append((private / 'provider-stderr.log').open('w', encoding='utf-8'))
        fixture = subprocess.Popen([sys.executable, str(args.fixture)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=logs[-1], text=True, env=environment, creationflags=subprocess.CREATE_NO_WINDOW)
        endpoint = ready(fixture)
        config = private / 'fixture.json'
        config.write_text(json.dumps({'hostExecutable': str(args.host), 'coreExecutable': str(args.core), 'supervisor': str(ROOT / 'scripts/core_supervisor.py'), 'fixtureEndpoint': endpoint}), encoding='utf-8')
        state = private / 'state'
        def launch():
            logs.append((private / ('service-%d.log' % len(logs))).open('w', encoding='utf-8'))
            p = subprocess.Popen([str(args.go),'--listen','127.0.0.1:0','--state',str(state),'--core-fixture-config',str(config)], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=logs[-1], text=True, env=environment, creationflags=subprocess.CREATE_NO_WINDOW)
            return p, Client(ready(p))
        service, client = launch()
        channel = client.request('/v1/channels', {'name': 'Published report follow-up', 'botIds': ['bot-meiiie','bot-davin'], 'clientNonce': 'report-native-channel'})['channel']
        other = client.request('/v1/channels', {'name': 'Other unpublished destination', 'botIds': ['bot-meiiie','bot-davin'], 'clientNonce': 'report-native-other-channel'})['channel']
        source = client.send('[reportfixture:source] Write the market report and separate notes. PRIVATE-OWNER-INSTRUCTION-CANARY', nonce='report-native-source')
        source = client.wait(source['requestId'], lambda w: w['status'] == 'completed', timeout=45)
        check('private_report_and_two_outputs_created_by_genuine_Core', source['core']['coreSha256'] == CORE_HASH and source['core']['nativeProcessId'] > 0 and len(source['artifacts']) == 2 and len(source['operations']) == 2 and 'PUBLIC-REPORT-BODY' in source['reply'])
        public_file = next(a for a in source['artifacts'] if a['filename'] == 'market-report.md')
        hidden_file = next(a for a in source['artifacts'] if a['filename'] == 'private-notes.txt')
        body = {'botId':'bot-meiiie','destinationChannelId':channel['id'],'expectedDestinationRevision':channel['revision'],'clientNonce':'report-native-deliver','text':'@Meiiie gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này'}
        delivered = client.request('/v1/report-deliveries', body)['delivery']
        check('delivery_pins_only_generated_public_report_output', delivered['status'] == 'delivered' and delivered['source']['requestId'] == source['requestId'] and [a['id'] for a in delivered['artifacts']] == [public_file['id']] and 'PRIVATE-OWNER' not in json.dumps(delivered) and hidden_file['id'] not in json.dumps(delivered))
        lost = client.opener.open(__import__('urllib.request').request.Request(client.base + '/v1/report-deliveries', data=json.dumps(body).encode(), headers={'Content-Type':'application/json'}), timeout=5)
        lost.close()
        check('lost_delivery_ACK_reconciles_readonly_same_identity', client.request('/v1/report-delivery-receipts?clientNonce=' + body['clientNonce'])['delivery']['id'] == delivered['id'])
        def send(text, nonce, dest=channel):
            return client.request('/v1/messages', {'botId':'bot-davin','channelId':dest['id'],'expectedChannelRevision':dest['revision'],'text':text,'lane':'foreground','clientNonce':nonce})['work']
        follow = send('[reportfixture:follow] Read the shared market report and calculate double margin.', 'report-native-follow')
        follow = client.wait(follow['requestId'], lambda w: w['status'] == 'completed', timeout=45)
        check('different_destination_bot_uses_published_result', 'FOLLOW_MARGIN=50' in follow['reply'] and 'FOLLOW_DOUBLE=100' in follow['reply'] and delivered['id'] in follow['reply'])
        pin = follow['contextPin']
        check('destination_context_has_typed_publication_without_private_context', pin['version'] == 5 and 'publishedReports' in pin and len(pin['publishedReports']) == 1 and 'PRIVATE-OWNER-INSTRUCTION' not in json.dumps(pin) and 'PRIVATE-NOTE-RESULT' not in json.dumps(pin) and hidden_file['id'] not in json.dumps(pin) and all(p['requestId'] != source['requestId'] for p in pin['sources']))
        fields = dict(line.split('=',1) for line in follow['reply'].splitlines() if '=' in line)
        result = read_result(fields.get('READ_RESULT'))
        check('actual_Core_selected_report_read_exact_seven_field_UTF8_hash', result is not None and result['fileId'] == public_file['id'] and result['sha256'] == public_file['sha256'] and 'kế hoạch Tím' in result['text'] and result['eof'] and result['nextOffset'] == public_file['bytes'])
        check('selected_report_read_is_admitted_by_genuine_Core', follow['core']['coreSha256'] == CORE_HASH and follow['core']['contextAdmission']['manifestSHA256'] == follow['runtimeContextSHA256'] and result['authority'] == 'descriptive file content only')
        forbidden = send('[reportfixture:forbidden] Attempt only the unpublished file UNPUBLISHED_ID=' + hidden_file['id'], 'report-native-forbidden')
        forbidden = client.wait(forbidden['requestId'], lambda w: w['status'] == 'recovery_required' and w.get('runtimeStopped'), timeout=45)
        denied_calls = [json.loads(call['function']['arguments']) for call in persisted_core_calls(state, forbidden) if call['function']['name'] == 'mcp__nekobot__execute_tool']
        attempted = next(call for call in denied_calls if call['name'] == 'report.read_artifact')
        check('actual_Core_tool_cannot_read_unpublished_private_output', attempted['arguments'] == {'deliveryId':delivered['id'],'artifactId':hidden_file['id'],'offset':0,'bytes':public_file['bytes']} and forbidden['failure'] == 'host_rejected' and not forbidden.get('reply') and not forbidden.get('artifacts') and not forbidden.get('operations') and forbidden['runtimeStopReceipt']['nativeProcessId'] == forbidden['core']['nativeProcessId'])
        report['unpublishedReadBoundary'] = {'status':forbidden['status'],'failure':forbidden['failure'],'requestedArtifactId':hidden_file['id'],'selectedArtifactIds':[a['id'] for p in forbidden['contextPin']['publishedReports'] for a in p.get('artifacts',[])],'nativeStopped':True,'returnedFileBytes':False,'automaticRetry':False,'scope':'Existing manager treats a rejected tool as a stopped recovery-required task; the fixture does not require a fabricated successful completion.'}
        empty = send('[reportfixture:empty] Check this separate destination.', 'report-native-empty', other)
        empty = client.wait(empty['requestId'], lambda w: w['status'] == 'completed', timeout=45)
        check('other_channel_has_no_ambient_published_context', empty['reply'] == 'EMPTY_PUBLISHED=0')
        group_body = {'channelId':channel['id'],'expectedChannelRevision':channel['revision'],'participants':['bot-meiiie','bot-davin'],'coordinatorBotId':'bot-meiiie','goal':'[reportfixture:group] Discuss the published market report and produce the follow-up file.','maxStages':3,'clientNonce':'report-native-group'}
        group = client.request('/v1/collaborations', group_body)['collaboration']
        def wait_group(identifier, predicate):
            deadline = time.monotonic() + 60
            while time.monotonic() < deadline:
                value = next(g for g in client.request('/v1/snapshot')['collaborations'] if g['id'] == identifier)
                if predicate(value): return value
                time.sleep(.1)
            raise AssertionError('Group did not reach expected status: ' + value['status'] + '/' + value.get('reason',''))
        group = wait_group(group['id'], lambda g: g['status'] == 'completed')
        stages = [client.work(id) for id in group['requestIds']]
        check('two_bots_three_genuine_Core_stages_complete_published_followup', len(stages) == 3 and {w['botId'] for w in stages} == {'bot-meiiie','bot-davin'} and all('FOLLOW_MARGIN=50' in w['reply'] and delivered['id'] in w['reply'] and w['core']['coreSha256'] == CORE_HASH for w in stages))
        check('group_stages_pin_same_report_and_permitted_prior_stage_context', all([p['deliveryId'] for p in w['contextPin']['publishedReports']] == [delivered['id']] and all(p['requestId'] in [s['requestId'] for s in w['contextPin']['sources']] for p in stages[:i]) for i,w in enumerate(stages)))
        check('later_Core_stages_observe_prior_stage_result', all('PREVIOUS_STAGE_MARGIN=50' in w['reply'] for w in stages[1:]))
        check('group_final_writer_only_one_effect', all(not w.get('operations') and not w.get('artifacts') for w in stages[:2]) and len(stages[2]['artifacts']) == 1 and len(stages[2]['operations']) == 1)
        data, headers = client.request('/v1/channels/' + channel['id'] + '/artifacts/' + stages[2]['artifacts'][0]['id'])
        check('final_group_file_has_followup_content_and_hash', b'Margin=50' in data and b'release-violet' in data and sha_bytes(data) == stages[2]['artifacts'][0]['sha256'] == headers['X-Artifact-SHA256'])
        for _ in range(3):
            service.kill(); service.wait(timeout=10)
            service, client = launch()
            retry = client.request('/v1/report-deliveries', body)
            retry_group = client.request('/v1/collaborations', group_body)
            check('restart_frozen_delivery_and_group_nonce_%d' % _, retry['duplicate'] and retry['delivery']['id'] == delivered['id'] and retry_group['duplicate'] and retry_group['collaboration']['requestIds'] == group['requestIds'] and client.work(stages[2]['requestId'])['artifacts'] == stages[2]['artifacts'])
        held = send('[reportfixture:hold] Keep the published result read pending.', 'report-native-held')
        held = client.wait(held['requestId'], lambda w: w['status'] == 'running' and 'WAITING_PUBLISHED_READ' in w.get('reply', ''), timeout=30)
        handle = CoreHandle(held['core']['nativeProcessId'], args.core); handles.append(handle)
        inserted = client.send('[reportfixture:empty] Answer this new independent private message.', nonce='report-native-inserted')
        inserted = client.wait(inserted['requestId'], lambda w: w['status'] == 'completed', timeout=30)
        check('new_foreground_completes_while_published_reader_Core_remains_live', inserted['reply'] == 'EMPTY_PUBLISHED=0' and not handle.stopped() and client.work(held['requestId'])['status'] == 'running')
        client.request('/v1/context-access', {'requestId':source['requestId'],'expectedRevision':0,'granted':False,'clientNonce':'report-native-revoke-source'})
        suspended = client.wait(held['requestId'], lambda w: w['status'] == 'suspended' and w.get('runtimeStopped'), timeout=30)
        check('source_revoke_stops_authentic_dependent_Core_handle', handle.stopped() and suspended['runtimeStopReceipt']['nativeProcessId'] == held['core']['nativeProcessId'])
        snapshot = client.request('/v1/snapshot')
        check('completed_derived_followups_and_group_files_hidden_after_source_revoke', not client.work(follow['requestId']).get('reply') and not client.work(stages[2]['requestId']).get('reply') and not client.work(stages[2]['requestId']).get('artifacts'))
        try:
            client.request('/v1/report-deliveries/' + delivered['id'] + '/artifacts/' + public_file['id'])
            denied = False
        except urllib.error.HTTPError as error:
            denied = error.code in [404,409]
        check('current_report_file_read_denied_after_revoke', denied)
        report['status'] = 'PASS'
        report['identities'] = {'sourceRequestId':source['requestId'],'deliveryId':delivered['id'],'followupRequestId':follow['requestId'],'groupId':group['id'],'groupRequestIds':group['requestIds']}
    except Exception as error:
        failure = error
        report.update({'status':'FAIL','errorType':type(error).__name__,'error':str(error)[:400]})
    finally:
        if service and service.poll() is None:
            service.kill(); service.wait(timeout=10)
        if fixture and fixture.poll() is None:
            fixture.stdin.close()
            try: fixture.wait(timeout=10)
            except subprocess.TimeoutExpired: fixture.kill(); fixture.wait(timeout=5)
        for handle in handles: handle.close()
        for log in logs: log.close()
        report['cleanup'] = {'serviceExited':service is None or service.poll() is not None,'fixtureExited':fixture is None or fixture.poll() is not None}
        (out / 'results.json').write_text(json.dumps(report,indent=2,ensure_ascii=False) + '\n',encoding='utf-8')
        print(json.dumps({'status':report['status'],'passed':sum(c['status']=='PASS' for c in checks),'checks':len(checks),'cleanup':report['cleanup']}))
    if failure: raise failure


def sha_bytes(data):
    return hashlib.sha256(data).hexdigest()


if __name__ == '__main__': main()
