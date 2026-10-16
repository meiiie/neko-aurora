"""Owned native Core profile fixtures; never reads an installed user's profile."""
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


class Model(BaseHandler):
    secret = 'fixture-profile-credential-canary'
    release = threading.Event()
    lock = threading.Lock()
    observations = []

    def do_POST(self):
        if self.path != '/v1/chat/completions':
            self.send_error(404)
            return
        matched = self.headers.get('Authorization') == 'Bearer ' + self.secret
        raw = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        body = json.loads(raw)
        manifest = admitted_fixture_context(body.get('messages', []))
        text = manifest['primaryMessage']['text'] if manifest else ''
        with self.lock:
            self.observations.append({'credentialMatches':matched,'modelMatches':body.get('model')=='profile-fixture-model','hasManifest':manifest is not None,'inputBytes':len(raw),'maxTokens':body.get('max_tokens'),'held':'[profile:hold]' in text})
        if not matched or body.get('model') != 'profile-fixture-model' or manifest is None:
            self.send_error(403)
            return
        self.send_response(200)
        self.send_header('Content-Type','text/event-stream')
        self.send_header('Connection','close')
        self.end_headers()
        try:
            self.chunk({'role':'assistant','content':'PROFILE_FIXTURE_ACCEPTED '})
            if '[profile:echo]' in text:
                for piece in [self.secret[:5],self.secret[5:17],self.secret[17:]]:
                    self.chunk({'content':piece})
            if '[profile:hold]' in text:
                self.wfile.flush()
                self.release.wait(50)
            self.chunk({'content':'isolated Core-owned credential source.'})
            self.chunk({},'stop')
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


def digest(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    for arg in ['go','host','core','go-tool','output']:
        parser.add_argument('--'+arg,required=True,type=pathlib.Path)
    parser.add_argument('--commit',required=True)
    args = parser.parse_args()
    out = args.output.resolve()
    assert out.is_relative_to(pathlib.Path('E:/CodexScratch/core-profile-bridge-20261012/root').resolve())
    out.mkdir(parents=True,exist_ok=False)
    private = out/'private-fixture'
    private.mkdir()
    home = private/'source-home'
    (home/'.neko-core').mkdir(parents=True)
    user = private/'service-home'
    user.mkdir()
    state = private/'state'
    server = ThreadingHTTPServer(('127.0.0.1',0),Model)
    thread = threading.Thread(target=server.serve_forever,daemon=True)
    thread.start()
    origin = 'http://127.0.0.1:%d' % server.server_address[1]
    source_file = home/'.neko-core/config.json'

    def write_profile(secret):
        value={'active_profile':'existing-fixture','profiles':{'existing-fixture':{'provider':'openai_compat','model':'profile-fixture-model','base_url':origin+'/v1','api_key':secret,'auth':'api_key'}}}
        source_file.write_text(json.dumps(value),encoding='utf-8')

    write_profile(Model.secret)
    initial_source_hash = digest(source_file)
    config = private/'trusted-sources.json'
    config.write_text(json.dumps({'version':1,'hostExecutable':str(args.host),'coreExecutable':str(args.core),'supervisor':str(ROOT/'scripts/core_supervisor.py'),'sources':[{'id':'existing-core-fixture','label':'Existing synthetic Core profile','kind':'fixture','credentialHome':str(home),'providerOrigin':origin}]}),encoding='utf-8')
    env={k:v for k,v in os.environ.items() if k.upper() in {'SYSTEMROOT','WINDIR','PATH','PATHEXT','COMSPEC','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'}}
    env.update({'USERPROFILE':str(user),'HOME':str(user),'TEMP':str(private),'TMP':str(private),'PYTHONDONTWRITEBYTECODE':'1'})
    checks=[]
    report={'status':'RUNNING','applicationCommit':args.commit,'coreSHA256':digest(args.core),'hostSHA256':digest(args.host),'controlSHA256':digest(args.go),'runnerSHA256':digest(__file__),'command':[sys.executable,*sys.argv],'checks':checks,'scope':'Synthetic Core1.7-shaped file profile, real Windows Go/Rust/bundled Core on owned loopback. No actual user profiles/credentials, OAuth/provider/cloud/Tauri/device acceptance.'}
    service=None
    handles=[]
    log=(private/'service.log').open('x',encoding='utf-8')

    def check(name,value):
        checks.append({'name':name,'status':'PASS' if value else 'FAIL'})
        assert value,name

    def start():
        process=subprocess.Popen([str(args.go),'--listen','127.0.0.1:0','--state',str(state),'--host-executable',str(args.host),'--core-profile-config',str(config)],stdout=subprocess.PIPE,stderr=log,text=True,env=env,creationflags=subprocess.CREATE_NO_WINDOW)
        client=Client(ready(process))
        client.opener.open(client.endpoint+'/').read()
        return process,client

    def snapshot(client):
        return client.request('/v1/snapshot')

    def approve(client,source,profile,nonce,seconds=90):
        limits={'maxInputBytes':64000,'maxOutputTokens':4096,'maxRequests':2,'maxSteps':2,'maxSeconds':seconds}
        body={'sourceId':source['id'],'expectedSourceRevision':source['revision'],'profileId':profile['id'],'profileRevision':profile['profileRevision'],'provider':profile['provider'],'model':profile['model'],'providerOrigin':profile['providerOrigin'],'channelId':'conversation-bot-meiiie','expectedChannelRevision':1,'botIds':['bot-meiiie'],**limits,'ttlSeconds':300,'approveExecution':True,'clientNonce':nonce}
        return client.request('/v1/core-profile-consents',body)['consent']

    def submit(client,consent,nonce,text='[profile:fast] Use existing synthetic profile',code=201):
        return client.request('/v1/messages',{'botId':'bot-meiiie','channelId':'conversation-bot-meiiie','expectedChannelRevision':1,'lane':'foreground','text':text,'clientNonce':nonce,'coreProfileSelection':{'consentId':consent['id'],'expectedConsentRevision':consent['revision']}},code=code)['work']

    try:
        head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip()
        clean=not subprocess.check_output(['git','status','--porcelain'],cwd=ROOT)
        build=subprocess.check_output([str(args.go_tool),'version','-m',str(args.go)]).decode()
        check('clean exact application control build',head==args.commit and clean and 'vcs.revision='+args.commit in build and 'vcs.modified=false' in build)
        service,client=start()
        source=client.request('/v1/core-profile-sources')['sources'][0]
        safe=json.dumps(source)
        check('discovery exposes no source root or credential and invokes no model',str(home) not in safe and Model.secret not in safe and not Model.observations)
        client.request('/v1/core-profiles?'+urllib.parse.urlencode({'sourceId':source['id'],'inspectionConsentId':'missing-fixture'}),code=403)
        check('missing inspection consent makes no provider call',not Model.observations)
        inspection=client.request('/v1/core-profile-inspection-consents',{'sourceId':source['id'],'expectedSourceRevision':source['revision'],'approveInspection':True,'ttlSeconds':300,'clientNonce':'profile-inspection-original'})['inspectionConsent']
        metadata=client.request('/v1/core-profiles?'+urllib.parse.urlencode({'sourceId':source['id'],'inspectionConsentId':inspection['id']}))
        profile=next(p for p in metadata['profiles'] if p['id']=='existing-fixture')
        check('Core catalog carries bounded supported profile without claiming auth readiness',profile['bridgeTransport']=='supported' and profile['credentialReadiness']=='not_checked' and profile['providerOrigin']==origin)
        check('inspection reads no model and leaves owned source configuration unchanged',not Model.observations and digest(source_file)==initial_source_hash and not (home/'.neko-core/tasks').exists())
        check('catalog contains no raw credential/path/environment names',Model.secret not in json.dumps(metadata) and str(home) not in json.dumps(metadata) and 'key_env' not in json.dumps(metadata))
        consent=approve(client,source,profile,'profile-execution-original')
        check('adopting explicit scope does not itself call provider',not Model.observations)
        work=submit(client,consent,'profile-request-original')
        completed=client.wait(work['requestId'],lambda w:w['status']=='completed',timeout=50)
        check('genuine native Core completes without credential reentry',completed.get('core',{}).get('nativeProcessId',0)>0 and 'PROFILE_FIXTURE_ACCEPTED' in completed['reply'])
        check('selected file credential and model consumed by Core rather than fixture fallback',len(Model.observations)==1 and Model.observations[0]['credentialMatches'] and Model.observations[0]['modelMatches'] and Model.observations[0]['hasManifest'])
        duplicate=submit(client,consent,'profile-request-original',code=200)
        check('same nonce returns original Work with no second provider request',duplicate['requestId']==completed['requestId'] and len(Model.observations)==1)
        pin=completed['coreProfilePin']
        check('public Work pins exact scope/profile/nonce without roots or credential',pin['profileRevision']==profile['profileRevision'] and pin['consentId']==consent['id'] and pin['clientNonce']=='profile-request-original' and str(home) not in json.dumps(pin) and Model.secret not in json.dumps(pin))
        mixed={'botId':'bot-meiiie','channelId':'conversation-bot-meiiie','expectedChannelRevision':1,'text':'No implicit account profile mix','lane':'foreground','clientNonce':'profile-mixed-denial','accountId':'account-fixture','accountRevision':1,'coreProfileSelection':{'consentId':consent['id'],'expectedConsentRevision':1}}
        client.request('/v1/messages',mixed,code=400)
        check('mixed account/profile selection denies before inference',len(Model.observations)==1)
        journal=(state/'transactions.jsonl').read_text(encoding='utf-8')
        check('API and journal contain no synthetic credential or private revision key path',Model.secret not in journal and str(home) not in journal and Model.secret not in json.dumps(snapshot(client)))
        check('task memory and budget are separate from credential home',any((state/'tasks').rglob('profile-bridge-budget-v1.json')) and not (home/'.neko-core/profile-bridge-budget-v1.json').exists())
        service.terminate();service.wait(timeout=8)
        service,client=start()
        restarted=next(w for w in snapshot(client)['work'] if w['requestId']==completed['requestId'])
        receipt=client.request('/v1/core-profile-receipts?'+urllib.parse.urlencode({'kind':'execution','clientNonce':'profile-execution-original'}))
        check('restart retains history and existing consent receipt without provider replay',restarted['reply']==completed['reply'] and receipt['found'] and len(Model.observations)==1)
        echo=submit(client,consent,'profile-request-echo','[profile:echo] Provider attempts split credential echo')
        echoed=client.wait(echo['requestId'],lambda w:w['status']=='completed',timeout=50)
        public=json.dumps(snapshot(client))+(state/'transactions.jsonl').read_text(encoding='utf-8')
        check('genuine split provider credential echo cannot enter public Work or journal',
              'PROFILE_FIXTURE_ACCEPTED' in echoed['reply'] and Model.secret not in public and len(Model.observations)==2)
        held=submit(client,consent,'profile-request-held','[profile:hold] Wait for source rotation')
        running=client.wait(held['requestId'],lambda w:w['status']=='running' and w.get('core',{}).get('nativeProcessId',0)>0 and 'PROFILE_FIXTURE_ACCEPTED' in w.get('reply',''),timeout=50)
        handle=CoreHandle(running['core']['nativeProcessId'],args.core)
        handles.append(handle)
        check('source rotation probe begins with genuine live Core HANDLE',not handle.stopped())
        write_profile('fixture-profile-rotated-credential')
        suspended=client.wait(held['requestId'],lambda w:w['status']=='suspended' and w.get('runtimeStopped') is True,timeout=25)
        check('credential rotation fences Work and observes genuine native stop',suspended['failure']=='core_profile_authority_changed' and handle.stopped())
        before=len(Model.observations)
        client.request('/v1/messages',{'botId':'bot-meiiie','channelId':'conversation-bot-meiiie','expectedChannelRevision':1,'text':'Old profile pin must stay denied','lane':'foreground','clientNonce':'profile-after-rotation','coreProfileSelection':{'consentId':consent['id'],'expectedConsentRevision':1}},code=409)
        check('old consent cannot silently repin rotated credential',len(Model.observations)==before)
        fresh_inspection=client.request('/v1/core-profile-inspection-consents',{'sourceId':source['id'],'expectedSourceRevision':source['revision'],'approveInspection':True,'ttlSeconds':300,'clientNonce':'profile-inspection-after-rotation'})['inspectionConsent']
        fresh_metadata=client.request('/v1/core-profiles?'+urllib.parse.urlencode({'sourceId':source['id'],'inspectionConsentId':fresh_inspection['id']}))
        fresh_profile=next(p for p in fresh_metadata['profiles'] if p['id']=='existing-fixture')
        check('explicit reinspection records changed keyed revision without reusing old grant',fresh_profile['profileRevision']!=profile['profileRevision'])
        Model.secret='fixture-profile-rotated-credential'
        fresh_consent=approve(client,source,fresh_profile,'profile-execution-after-rotation')
        held=submit(client,fresh_consent,'profile-request-revoke','[profile:hold] Wait for explicit revoke')
        running=client.wait(held['requestId'],lambda w:w['status']=='running' and w.get('core',{}).get('nativeProcessId',0)>0 and 'PROFILE_FIXTURE_ACCEPTED' in w.get('reply',''),timeout=50)
        handle=CoreHandle(running['core']['nativeProcessId'],args.core)
        handles.append(handle)
        client.request('/v1/core-profile-consents/control',{'consentId':fresh_consent['id'],'expectedRevision':fresh_consent['revision'],'action':'revoke','clientNonce':'profile-explicit-revoke'})
        stopped=client.wait(held['requestId'],lambda w:w['status']=='suspended' and w.get('runtimeStopped') is True,timeout=25)
        check('explicit consent revoke stops genuine Core and retains original pin',handle.stopped() and stopped['coreProfilePin']['consentRevision']==1)
        receipt=client.request('/v1/core-profile-receipts?'+urllib.parse.urlencode({'kind':'execution','clientNonce':'profile-execution-after-rotation'}))
        check('revoked consent receipt remains historical with no new grant',receipt['found'] and not receipt['active'] and receipt['consent']['revision']==1)
        expiry_consent=approve(client,source,fresh_profile,'profile-execution-expiry',seconds=30)
        held=submit(client,expiry_consent,'profile-request-expiry','[profile:hold] Wait for bound request deadline')
        running=client.wait(held['requestId'],lambda w:w['status']=='running' and w.get('core',{}).get('nativeProcessId',0)>0 and 'PROFILE_FIXTURE_ACCEPTED' in w.get('reply',''),timeout=30)
        handle=CoreHandle(running['core']['nativeProcessId'],args.core)
        handles.append(handle)
        stopped=client.wait(held['requestId'],lambda w:w['status']=='suspended' and w.get('runtimeStopped') is True,timeout=45)
        check('per-request deadline fences live native Core without renewing consent',handle.stopped() and stopped['coreProfilePin']['expiresAtUnixMs']<=int(time.time()*1000))
        Model.release.set()
        before=len(Model.observations)
        time.sleep(.15)
        check('late provider release cannot publish suspended output or spend another request',len(Model.observations)==before and next(w for w in snapshot(client)['work'] if w['requestId']==held['requestId'])['status']=='suspended')
        report['observations']=list(Model.observations)
        report['status']='PASS'
    except BaseException as error:
        report['status']='FAIL'
        report['failureType']=type(error).__name__
        report['failure']=str(error)
        (out/'failure.txt').write_text(traceback.format_exc(),encoding='utf-8')
    finally:
        report['observations']=list(Model.observations)
        Model.release.set()
        if service is not None and service.poll() is None:
            service.terminate()
            try:service.wait(timeout=8)
            except subprocess.TimeoutExpired:service.kill();service.wait(timeout=5)
        for handle in handles:handle.close()
        log.close()
        server.shutdown();server.server_close()
        (out/'result.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report))
    return 0 if report['status']=='PASS' else 1


if __name__=='__main__':
    raise SystemExit(main())
