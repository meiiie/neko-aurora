"""Actual loopback principal/device boundary with genuine Go/Rust/Core processes."""
import argparse
import base64
import hashlib
import http.cookiejar
import json
import os
import pathlib
import platform
import queue
import subprocess
import sys
import threading
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'scripts'))
from e2e_tag_collaboration_daily import CoreHandle


def sha(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def sha_bytes(value):
    return hashlib.sha256(value).hexdigest()


def ready(process):
    q = queue.Queue()
    threading.Thread(target=lambda: q.put(process.stdout.readline().strip()), daemon=True).start()
    line = q.get(timeout=25)
    assert line.startswith('READY http://127.0.0.1:')
    return line[6:]


class Client:
    def __init__(self, endpoint):
        self.endpoint = endpoint
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))
        self.binding = None

    def request(self, path, value=None, code=200, raw=False):
        body = None if value is None else json.dumps(value).encode()
        headers = {'Content-Type':'application/json', 'Origin':self.endpoint}
        if self.binding and path not in ['/v1/session','/v1/fixture-sessions']:
            headers['X-Neko-Session-Binding'] = self.binding
        request = urllib.request.Request(self.endpoint + path, data=body, headers=headers)
        try:
            response = self.opener.open(request, timeout=15)
        except urllib.error.HTTPError as e:
            assert e.code == code, (path, e.code, code)
            return e.read().decode()
        except ConnectionResetError as e:
            raise AssertionError('transport reset at '+path) from e
        assert response.status == code, (path, response.status, code)
        result = response.read()
        if raw: return result
        value = json.loads(result)
        if path in ['/v1/session','/v1/fixture-sessions'] and value.get('mode')=='fixture': self.binding=value['sessionBinding']
        return value

    def wait(self, request_id, predicate, timeout=40):
        until = time.monotonic() + timeout
        while time.monotonic() < until:
            value = next((w for w in self.request('/v1/snapshot')['work'] if w['requestId'] == request_id), None)
            if value is not None and predicate(value):
                return value
            time.sleep(.1)
        raise AssertionError('request did not reach expected state: ' + request_id)

    def stream(self, after):
        binding, sequence = after.split(':')
        response = self.opener.open(self.endpoint+'/v1/events?'+urllib.parse.urlencode({'sessionBinding':binding,'after':sequence}),timeout=10)
        events, finished = [], threading.Event()
        def read():
            try:
                while True:
                    line = response.readline()
                    if not line: break
                    if line.startswith(b'data: '): events.append(json.loads(line[6:]))
            except (OSError,ValueError):
                pass
            finally:
                finished.set()
                response.close()
        threading.Thread(target=read,daemon=True).start()
        return response,events,finished


def main():
    p = argparse.ArgumentParser()
    for name in ['go','host','core','go-tool','fixture','output']:
        p.add_argument('--'+name, required=True, type=pathlib.Path)
    p.add_argument('--commit', required=True)
    args = p.parse_args()
    out = args.output.resolve()
    assert out.is_relative_to(pathlib.Path('E:/CodexScratch/principal-device-boundary-20261012').resolve())
    out.mkdir(parents=True, exist_ok=False)
    private = out / 'private-fixture'
    private.mkdir()
    checks, logs, processes, handles = [], [], [], []
    head = subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip()
    report = {'status':'RUNNING','applicationCommit':args.commit,'sourceHeadAtRun':head,'sourceWorkingTreeAtRun':bool(subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)),'runnerSHA256':sha(__file__),'fixtureSHA256':sha(args.fixture),'binaryHashes':{n:sha(f) for n,f in [('go',args.go),('host',args.host),('core',args.core)]},'command':[sys.executable,*sys.argv],'environment':{'python':platform.python_version(),'platform':platform.platform()},'checks':checks,'scope':'Isolated fixture actors/devices, current shared ACL and opaque sessions on loopback with genuine Windows Go/Rust/Core. Not production login/OAuth/device keys, remote transport, external effects, paid provider, model quality, Tauri or phone.'}
    def check(name, condition):
        checks.append({'name':name,'status':'PASS' if condition else 'FAIL'})
        assert condition, name
    env = {k:v for k,v in os.environ.items() if k.upper() in {'SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'}}
    env.update({'TEMP':str(private),'TMP':str(private),'PYTHONDONTWRITEBYTECODE':'1'})
    failure = None
    try:
        info = subprocess.check_output([str(args.go_tool),'version','-m',str(args.go)]).decode()
        check('clean exact control build',head==args.commit and not report['sourceWorkingTreeAtRun'] and 'vcs.revision='+args.commit in info and 'vcs.modified=false' in info)
        logs.append((private/'provider.log').open('w',encoding='utf-8'))
        fixture = subprocess.Popen([sys.executable,str(args.fixture)],stdout=subprocess.PIPE,stderr=logs[-1],text=True,env=env,creationflags=subprocess.CREATE_NO_WINDOW)
        processes.append(fixture)
        fixture_endpoint = ready(fixture)
        config = private/'core-fixture.json'
        config.write_text(json.dumps({'hostExecutable':str(args.host),'coreExecutable':str(args.core),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'fixtureEndpoint':fixture_endpoint}),encoding='utf-8')
        identity_config = private/'identity-fixture.json'
        identity_config.write_text(json.dumps({'version':1,'fixtureOnly':True,'authorityId':'fixture-office','spaceId':'local-space','sessionTTLSeconds':300,'actors':[{'actorId':actor,'deviceId':device,'deviceRevision':1} for actor,device in [('local-owner','device-alice'),('fixture-bob','device-bob'),('fixture-bob','device-bob-two'),('fixture-reader','device-reader'),('fixture-eve','device-eve')]]}),encoding='utf-8')
        state, tickets = private/'state', private/'tickets-private.json'
        logs.append((private/'legacy-owner-service.log').open('w',encoding='utf-8'))
        producer = subprocess.Popen([str(args.go),'--listen','127.0.0.1:0','--state',str(state),'--host-executable',str(args.host),'--core-fixture-config',str(config)],stdout=subprocess.PIPE,stderr=logs[-1],text=True,env=env,creationflags=subprocess.CREATE_NO_WINDOW)
        processes.append(producer)
        legacy = Client(ready(producer))
        legacy.opener.open(legacy.endpoint+'/').read()
        legacy_input = {'botId':'bot-meiiie','text':'[principal:fast] Báo cáo ALICE-PRIVATE-CANARY','lane':'foreground','clientNonce':'principal-alice-private'}
        private_work = legacy.request('/v1/messages',legacy_input,code=201)['work']
        old_completed = legacy.wait(private_work['requestId'],lambda w:w['status']=='completed')
        stored_legacy = [json.loads(line)['event']['work'] for line in (state/'transactions.jsonl').read_text(encoding='utf-8').splitlines() if json.loads(line)['event'].get('work',{}).get('requestId')==private_work['requestId']][-1]
        check('genuine legacy owner producer has no requester execution grant',not stored_legacy.get('requesterPin') and stored_legacy['runtimeContext']['version']==1)
        producer.terminate()
        producer.wait(timeout=5)
        logs.append((private/'service.log').open('w',encoding='utf-8'))
        service = subprocess.Popen([str(args.go),'--listen','127.0.0.1:0','--state',str(state),'--host-executable',str(args.host),'--core-fixture-config',str(config),'--fixture-identity-config',str(identity_config),'--fixture-session-output',str(tickets)],stdout=subprocess.PIPE,stderr=logs[-1],text=True,env=env,creationflags=subprocess.CREATE_NO_WINDOW)
        processes.append(service)
        endpoint = ready(service)
        legacy.endpoint = endpoint
        legacy.request('/v1/snapshot',code=401)
        check('legacy owner cookie is not accepted as a fixture principal',True)
        bootstrap = json.loads(tickets.read_text(encoding='utf-8'))
        anonymous = Client(endpoint)
        request = anonymous.opener.open(endpoint+'/')
        check('public page does not mint owner session',not request.headers.get('Set-Cookie') and not list(anonymous.jar))
        anonymous.request('/v1/snapshot',code=401)
        check('anonymous snapshot denied',True)
        clients = {}
        for row in bootstrap['sessions']:
            c = Client(endpoint)
            c.request('/v1/fixture-sessions',{'ticket':row['ticket']})
            clients[row['deviceId']] = c
        alice, bob, bob_two, reader, eve = (clients[n] for n in ['device-alice','device-bob','device-bob-two','device-reader','device-eve'])
        bob_session = bob.request('/v1/session')
        bob_two_session = bob_two.request('/v1/session')
        check('same actor cache namespace and distinct session binding',bob_session['actorId']=='fixture-bob' and bob_session['cacheNamespace']==bob_two_session['cacheNamespace'] and bob_session['sessionBinding']!=bob_two_session['sessionBinding'])
        legacy_ack = alice.request('/v1/messages',legacy_input)
        check('verified owner migration returns legacy receipt without new work or grant',legacy_ack['duplicate'] and legacy_ack['work']['requestId']==private_work['requestId'] and not legacy_ack['work'].get('requesterPin') and legacy_ack['work']['status']=='completed')
        acl = {'version':1,'authorityId':'fixture-office','members':[{'actorId':'fixture-bob','role':'writer'},{'actorId':'fixture-reader','role':'reader'}]}
        channel = alice.request('/v1/channels',{'name':'Fixture shared office','botIds':['bot-meiiie','bot-davin'],'humanAcl':acl,'clientNonce':'principal-shared-channel'})['channel']
        channel_id, revision = channel['id'], channel['revision']
        published=alice.request('/v1/report-deliveries',{'botId':'bot-meiiie','destinationChannelId':channel_id,'expectedDestinationRevision':revision,'clientNonce':'principal-publish-legacy-report','text':'@Meiiie gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này'},code=201)['delivery']
        check('owner publishes completed private result without source prompt',published['status']=='delivered' and 'ALICE-PRIVATE-CANARY' not in published.get('reply',''))
        reader_report=next(d for d in reader.request('/v1/snapshot')['reportDeliveries'] if d['id']==published['id'])
        check('member receives public report projection without private source',reader_report['active'] and reader_report['reply']==published['reply'] and 'source' not in reader_report)
        def body(nonce,text='[principal:fast] Shared follow-up'):
            return {'botId':'bot-davin','channelId':channel_id,'expectedChannelRevision':revision,'text':text,'lane':'foreground','clientNonce':nonce}
        created = bob.request('/v1/messages',body('principal-bob-intake'),code=201)['work']
        completed = bob.wait(created['requestId'],lambda w:w['status']=='completed')
        check('real Core uses Bob sender while keeping workspace owner','FIXTURE_SENDER=fixture-bob' in completed.get('reply','') and 'WORKSPACE_OWNER=local-owner' in completed.get('reply','') and 'MANIFEST_VERSION=2' in completed.get('reply',''))
        visible = bob.request('/v1/snapshot')
        flat = json.dumps(visible)
        forbidden = {'core','runtimeAttemptBinding','runtimeStopReceipt','contextPin','runtimeContext','accountPin','accountLease','botProfilePin','executorPin','operations','currentAttemptLease'}
        check('member snapshot excludes private history and nested execution data','ALICE-PRIVATE-CANARY' not in flat and private_work['requestId'] not in flat and all(not forbidden.intersection(w) for w in visible['work']) and all(not b.get('description') for b in visible['bots']))
        reader.request('/v1/messages',body('principal-reader-denied'),code=403)
        eve.request('/v1/messages',body('principal-eve-denied'),code=403)
        bob.request('/v1/messages',{**body('principal-spoof-denied'),'actorId':'local-owner'},code=400)
        bob.request('/v1/messages',{'botId':'bot-meiiie','text':'Do not import Alice history','lane':'foreground','clientNonce':'principal-private-denied'},code=403)
        check('reader outsider identity spoof and private lane rejected',True)
        for path in ['/v1/connections','/v1/plugin-catalog','/v1/adapters/catalog','/v1/conversations']:
            bob.request(path,code=403)
        check('member cannot inspect owner administration',True)
        original_binding = bob.binding
        before_ids = {w['requestId'] for w in alice.request('/v1/snapshot')['work']}
        bob.binding = alice.request('/v1/session')['sessionBinding']
        bob.request('/v1/messages',body('principal-stale-browser-intent','OLD_ALICE_INTENT_CANARY'),code=409)
        bob.binding = None
        bob.request('/v1/messages',body('principal-unbound-browser-intent'),code=409)
        bob.binding = original_binding
        after_ids = {w['requestId'] for w in alice.request('/v1/snapshot')['work']}
        check('changed browser principal cannot execute another actors pending intent',before_ids==after_ids)
        duplicate = bob_two.request('/v1/messages',body('principal-bob-intake'))
        check('second same actor device keeps original obligation',duplicate['duplicate'] and duplicate['work']['requestId']==created['requestId'])
        bob_two.request('/v1/messages',body('principal-bob-intake','Changed text'),code=409)
        alice_same = alice.request('/v1/messages',body('principal-bob-intake'),code=201)['work']
        check('same nonce from another actor is independent',alice_same['requestId']!=created['requestId'])
        file_work = bob.request('/v1/messages',body('principal-bob-report','[principal:artifact] Write a shared report as the verified participant'),code=201)['work']
        file_work = bob.wait(file_work['requestId'],lambda w:w['status']=='completed' and w.get('artifacts'))
        artifact = file_work['artifacts'][0]
        file_path = '/v1/channels/'+channel_id+'/artifacts/'+artifact['id']
        data = reader.request(file_path,raw=True)
        check('real Core output is downloadable by current shared reader',sha_bytes(data)==artifact['sha256'] and b'fixture-bob' in data)
        sync_path = '/v1/local-sync?'+urllib.parse.urlencode({'botId':'bot-davin','channelId':channel_id})
        envelope = bob.request(sync_path)
        cursor = base64.urlsafe_b64encode(json.dumps({'version':1,'instanceId':envelope['instanceId'],'scopeId':envelope['scopeId'],'sequence':envelope['to']},separators=(',',':')).encode()).decode().rstrip('=')
        bob.request(sync_path+'&cursor='+urllib.parse.quote(cursor))
        bob_two.request(sync_path+'&cursor='+urllib.parse.quote(cursor),code=409)
        eve.request(sync_path,code=403)
        check('cursor is principal device session and channel scoped',True)
        event_cursor = bob_session['sessionBinding']+':'+str(bob.request('/v1/snapshot')['sequence'])
        bob_two.request('/v1/events?'+urllib.parse.urlencode({'sessionBinding':bob_session['sessionBinding'],'after':event_cursor.split(':')[1]}),code=409)
        bob.request('/v1/events?after=0',code=409)
        check('SSE rejects another session cursor and bare journal offset',True)
        stream, streamed, stream_finished = bob.stream(event_cursor)
        held_body = body('principal-bob-held','[principal:hold] Keep running until device revoke')
        held = bob.request('/v1/messages',held_body,code=201)['work']
        live = alice.wait(held['requestId'],lambda w:w['status']=='running' and w.get('core') and w.get('reply'))
        handle = CoreHandle(live['core']['nativeProcessId'], args.core)
        handles.append(handle)
        check('held genuine Core process is live',not handle.stopped())
        before_pin = live['requesterPin']
        alice.request('/v1/fixture-devices/control',{'deviceId':'device-bob','action':'revoke','expectedRevision':1})
        bob.request('/v1/session',code=401)
        stopped = alice.wait(held['requestId'],lambda w:w['status']=='suspended' and w.get('runtimeStopped'))
        check('device revoke stops authentic Core and retains original pin',handle.stopped() and stopped['runtimeStopReceipt']['nativeProcessId']==live['core']['nativeProcessId'] and stopped['requesterPin']==before_pin)
        check('completed shared file survives submitting device revoke',reader.request(file_path,raw=True)==data)
        check('revoked principal stream closes',stream_finished.wait(3))
        check('stream frames retain caller binding without private data',streamed and all(v['sessionBinding']==bob_session['sessionBinding'] and 'ALICE-PRIVATE-CANARY' not in json.dumps(v) for v in streamed))
        old_ack = bob_two.request('/v1/messages',held_body)
        check('valid second device receipt cannot regrant revoked original task',old_ack['duplicate'] and old_ack['work']['requestId']==held['requestId'] and old_ack['work']['status']=='suspended')
        rotated = alice.request('/v1/fixture-devices/control',{'deviceId':'device-bob-two','action':'rotate','expectedRevision':1})
        bob_two.request('/v1/session',code=401)
        bob_new = Client(endpoint)
        bob_new.request('/v1/fixture-sessions',{'ticket':rotated['ticket']})
        new_status = bob_new.request('/v1/session')
        check('rotation preserves actor namespace and changes binding',new_status['cacheNamespace']==bob_session['cacheNamespace'] and new_status['sessionBinding']!=bob_two_session['sessionBinding'])
        bob_new.request(sync_path+'&cursor='+urllib.parse.quote(cursor),code=409)
        renewed_ack = bob_new.request('/v1/messages',held_body)
        check('renewal cannot mutate original pin or request identity',renewed_ack['duplicate'] and renewed_ack['work']['requestId']==held['requestId'])
        new_acl = {**acl,'members':[{'actorId':'fixture-reader','role':'reader'}]}
        updated = alice.request('/v1/channels',{'id':channel_id,'name':channel['name'],'botIds':channel['botIds'],'expectedRevision':revision,'humanAcl':new_acl,'clientNonce':'principal-remove-bob'})['channel']
        check('human ACL changes advance audience revision',updated['revision']==revision+1)
        bob_new.request('/v1/messages',held_body,code=403)
        bob_new.request(sync_path,code=403)
        check('current ACL rejects receipt replay and read after membership removal',True)
        observations = json.loads(urllib.request.urlopen(fixture_endpoint.removesuffix('/v1')+'/observations').read())
        bob_seen = [v for v in observations if v['senderId']=='fixture-bob']
        check('model fixture independently observed truthful sender with no private canary',bob_seen and all(v['manifestVersion']==2 and v['requesterActorId']=='fixture-bob' and v['ownerUserId']=='local-owner' and not v['privateCanaryPresent'] for v in bob_seen))
        report['identities'] = {'channelId':channel_id,'bobRequestId':created['requestId'],'revokedRequestId':held['requestId'],'coreProcessId':live['core']['nativeProcessId']}
        report['status'] = 'PASS'
    except BaseException as e:
        report['status'] = 'FAIL'
        failure = {'type':type(e).__name__,'message':str(e)[:500]}
        report['failure'] = failure
        (out/'failure-traceback.log').write_text(traceback.format_exc(),encoding='utf-8')
        report['processExitCodesBeforeCleanup']=[v.poll() for v in processes]
    finally:
        for process in reversed(processes):
            if process.poll() is None:
                process.terminate()
                try: process.wait(timeout=5)
                except subprocess.TimeoutExpired: process.kill(); process.wait(timeout=5)
        for handle in handles:
            handle.close()
        for log in logs:
            log.close()
        report['cleanup'] = {'ownedProcessesExited':all(v.poll() is not None for v in processes),'liveCoreHandlesStopped':all(h.stopped() for h in handles) if not handles else 'checked before close; native revoke evidence above'}
        (out/'results.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
        print(json.dumps({'status':report['status'],'passed':sum(x['status']=='PASS' for x in checks),'failed':sum(x['status']=='FAIL' for x in checks),'failure':failure,'scope':report['scope']}))
    raise SystemExit(report['status']!='PASS')


if __name__ == '__main__':
    main()
