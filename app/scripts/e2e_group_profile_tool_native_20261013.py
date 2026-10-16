"""Finite group profile composition using authored credentials and real native Core."""
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
PINS = {'host':'a1693c4f3309c86e29b348ffc913c888cae3f8fd2bdf04bcac418e107a6654d2', 'core':'356247616082e613abd99b97838fd533f5e14ab64b07f480d78ae799d10648bf'}
FIXTURE_SECRET = 'published-finite-group-profile-fixture'
FILE_TEXT = '# Finite profile group\nThree original stages and one final writer.\n'


def sha(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    for name in ('source', 'go', 'host', 'core', 'go-tool', 'output'):
        parser.add_argument('--' + name, type=pathlib.Path, required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--expected-host-sha', required=True)
    parser.add_argument('--text-only', action='store_true')
    parser.add_argument('--rotation-only', action='store_true')
    args = parser.parse_args()
    assert len(args.expected_host_sha)==64 and all(ch in '0123456789abcdef' for ch in args.expected_host_sha)
    PINS['host']=args.expected_host_sha
    source = args.source.resolve()
    out = args.output.resolve()
    assert out.is_relative_to(pathlib.Path('E:/CodexScratch/group-core-profile-20261013/tool-validation').resolve())
    out.mkdir(parents=True, exist_ok=False)
    private = out / 'private-fixture'
    private.mkdir()
    sys.path.insert(0, str(source / 'scripts'))
    from e2e_principal_device import Client, ready
    from e2e_tag_collaboration_daily import CoreHandle
    from fixture_provider import Handler as BaseHandler, admitted_fixture_context

    # Named lookups only; neither enumerate nor inherit provider/proxy/credential variables.
    allowed = ('SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS')
    env = {key: os.environ[key] for key in allowed if key in os.environ}
    for key in ('HOME','USERPROFILE','APPDATA','LOCALAPPDATA','TEMP','TMP'):
        path = private / key.lower()
        path.mkdir()
        env[key] = str(path)
    env.update({'PYTHONDONTWRITEBYTECODE':'1','GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'NUL','GIT_OPTIONAL_LOCKS':'0'})
    dependencies = ('scripts/e2e_principal_device.py','scripts/e2e_tag_collaboration_daily.py','scripts/fixture_provider.py','scripts/core_supervisor.py')
    checks = []
    report = {'status':'RUNNING','applicationCommit':args.commit,'command':[sys.executable,*sys.argv], 'runnerSHA256':sha(__file__), 'binaryHashes':{name:sha(getattr(args,name)) for name in ('go','host','core')}, 'dependencyHashes':{name:sha(source/name) for name in dependencies},'environment':{'names':sorted(env),'python':sys.version.split()[0], 'scope':'Only named OS fields and owned home/temp paths; no real credential/provider/proxy inheritance.'},'checks':checks,'scope':'Real Windows Go/Rust/pinned Core with owned authored profile and loopback model fixture. Not real credentials/providers, model quality, Tauri GUI, phone or cloud acceptance.', 'recipe':'Scoped profile permission mediation: genuine three-stage final artifact and native authority interruption'}
    service = None
    logs, handles = [], []
    release = threading.Event()
    observations, model_failures = [], []
    lock = threading.Lock()

    class Model(BaseHandler):
        def do_POST(self):
            if self.path != '/v1/chat/completions':
                self.send_error(404)
                return
            length = int(self.headers.get('Content-Length', '0'))
            if not 0 < length <= 64000:
                self.send_error(413)
                return
            raw = self.rfile.read(length)
            body = json.loads(raw)
            admitted = admitted_fixture_context(body.get('messages', []))
            matched = self.headers.get('Authorization') == 'Bearer ' + FIXTURE_SECRET
            if not matched or body.get('model') != 'finite-group-fixture-model' or admitted is None:
                model_failures.append('credential_model_or_admission_mismatch')
                self.send_error(403)
                return
            derived = []
            for item in admitted.get('sources', []):
                if item.get('kind') == 'derived':
                    try:
                        derived.append(json.loads(item['span']['text']))
                    except ValueError:
                        pass
            stage_data = next((value for value in derived if value.get('kind') == 'generated-collaboration-stage'), None)
            if stage_data is None:
                model_failures.append('missing_group_stage')
                self.send_error(409)
                return
            stage = stage_data['stage']
            peers = [value for value in derived if value.get('kind') == 'pinned-prior-turn']
            tools = [m for m in body['messages'] if m.get('role') == 'tool']
            text = admitted['primaryMessage']['text']
            held = '[finite-native:hold]' in text
            with lock:
                observations.append({'groupId':stage_data['collaborationId'],'stage':stage,'credentialMatched':matched,'modelMatched':True,'admitted':True,'inputBytes':len(raw),'maxTokens':body.get('max_tokens'),'peerCount':len(peers),'peerRequestIds':[p['requestId'] for p in peers],'hasToolResult':bool(tools),'held':held})
            self.send_response(200)
            self.send_header('Content-Type','text/event-stream')
            self.send_header('Connection','close')
            self.end_headers()
            try:
                if len(peers) != stage - 1:
                    raise AssertionError('actual_admitted_peer_count')
                if held:
                    self.chunk({'role':'assistant','content':'FINITE_GROUP_HELD '})
                    self.wfile.flush()
                    release.wait(45)
                    self.chunk({'content':'late owned fixture release'})
                elif not tools and not args.text_only and (stage == 3 or '[finite-native:deny-1]' in text and stage == 1 or '[finite-native:deny-2]' in text and stage == 2):
                    call = {'name':'artifact.write_text','arguments':{'filename':'finite-profile-group.md','text':FILE_TEXT}}
                    self.chunk({'role':'assistant','tool_calls':[{'index':0,'id':'finite-profile-final-writer','type':'function','function':{'name':'mcp__nekobot__execute_tool','arguments':json.dumps(call)}}]})
                    self.chunk({},'tool_calls')
                    self.wfile.write(b'data: [DONE]\n\n')
                    self.wfile.flush()
                    return
                else:
                    self.chunk({'role':'assistant','content':'FINITE_PROFILE_STAGE_%d peers=%d' % (stage,len(peers))})
                self.chunk({},'stop')
                self.wfile.write(b'data: [DONE]\n\n')
                self.wfile.flush()
            except (BrokenPipeError,ConnectionResetError):
                pass
            except Exception as error:
                model_failures.append(type(error).__name__ + ':' + str(error))

    server = ThreadingHTTPServer(('127.0.0.1',0), Model)
    server.daemon_threads = True
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    origin = 'http://127.0.0.1:%d' % server.server_port
    home = private/'credential-fixture-home'
    (home/'.neko-core').mkdir(parents=True)
    source_file = home/'.neko-core/config.json'
    def write_profile(secret):
        source_file.write_text(json.dumps({'active_profile':'finite-fixture','profiles':{'finite-fixture':{'provider':'openai_compat','model':'finite-group-fixture-model','base_url':origin+'/v1','api_key':secret,'auth':'api_key'}}}),encoding='utf-8')
    write_profile(FIXTURE_SECRET)
    initial_hash = sha(source_file)
    config = private/'trusted-sources.json'
    config.write_text(json.dumps({'version':1,'hostExecutable':str(args.host),'coreExecutable':str(args.core),'supervisor':str(source/'scripts/core_supervisor.py'),'sources':[{'id':'finite-core-fixture','label':'Authored finite group profile','kind':'fixture','credentialHome':str(home),'providerOrigin':origin}]}),encoding='utf-8')
    state = private/'state'

    def check(name, condition):
        checks.append({'name':name,'status':'PASS' if condition else 'FAIL'})
        assert condition, name

    def stop():
        nonlocal service
        if service is not None and service.poll() is None:
            service.terminate()
            try:
                service.wait(timeout=8)
            except subprocess.TimeoutExpired:
                service.kill()
                service.wait(timeout=5)
        service = None

    def start():
        nonlocal service
        log = (private/('service-%d.log' % len(logs))).open('x',encoding='utf-8')
        logs.append(log)
        command = [str(args.go),'--listen','127.0.0.1:0','--state',str(state),'--host-executable',str(args.host),'--core-profile-config',str(config)]
        service = subprocess.Popen(command,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=log,text=True,encoding='utf-8',env=env,creationflags=subprocess.CREATE_NO_WINDOW)
        report.setdefault('launches',[]).append({'command':command,'pid':service.pid})
        client = Client(ready(service))
        client.opener.open(client.endpoint+'/').read()
        return client

    def get_group(client, identifier):
        return next(g for g in client.request('/v1/collaborations')['collaborations'] if g['id']==identifier)

    def wait_group(client, identifier):
        until = time.monotonic()+90
        while time.monotonic()<until:
            group = get_group(client,identifier)
            if group['status'] in ('completed','failed','needs_owner','suspended','cancelled'):
                return group
            time.sleep(.1)
        raise AssertionError('group_did_not_finish')

    def approve_group(client, catalog_source, profile, label, hold=False, seconds=300):
        channel = client.request('/v1/channels',{'name':'Finite native '+label,'botIds':['bot-davin','bot-meiiie'],'clientNonce':'finite-native-channel-'+label})['channel']
        marker='[finite-native:deny-1]' if label=='deny1' else '[finite-native:deny-2]' if label=='deny2' else '[finite-native:hold]' if hold else '[finite-native:complete]'
        intent = {'version':1,'channelId':channel['id'],'expectedChannelRevision':channel['revision'],'participants':['bot-davin','bot-meiiie'],'coordinatorBotId':'bot-meiiie','goal':marker+(' Compare the two proposals and return the final conclusion.' if args.text_only else ' Compare the two proposals and write the final group file.'),'maxStages':3,'clientNonce':'finite-native-group-'+label}
        body = {'purpose':'finite_collaboration_v1','groupIntent':intent,'sourceId':catalog_source['id'],'expectedSourceRevision':catalog_source['revision'],'profileId':profile['id'],'profileRevision':profile['profileRevision'],'provider':profile['provider'],'model':profile['model'],'providerOrigin':profile['providerOrigin'],'channelId':channel['id'],'expectedChannelRevision':channel['revision'],'botIds':intent['participants'],'maxInputBytes':64000,'maxOutputTokens':4096,'maxRequests':6,'maxSteps':6,'maxSeconds':seconds,'ttlSeconds':600,'approveExecution':True,'clientNonce':'finite-native-consent-'+label}
        before = len(observations)
        consent = client.request('/v1/core-profile-consents',body)['consent']
        receipt = client.request('/v1/core-profile-receipts?'+urllib.parse.urlencode({'kind':'group_execution','clientNonce':body['clientNonce']}))
        canonical = json.dumps({'version':1,'purpose':body['purpose'],'intent':intent},sort_keys=True,separators=(',',':')).encode()
        check(label+'_consent_exact_purpose_hash_receipt_without_provider',consent['version']==2 and consent['groupIntent']==intent and consent['groupIntentSha256']==hashlib.sha256(canonical).hexdigest() and receipt['found'] and receipt['active'] and receipt['input']==body and len(observations)==before)
        selected = {k:v for k,v in intent.items() if k!='version'}
        selected['coreProfileSelection']={'consentId':consent['id'],'expectedConsentRevision':consent['revision']}
        group = client.request('/v1/collaborations',selected,code=201)['collaboration']
        return channel,consent,body,selected,group

    started = time.monotonic()
    try:
        head = subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,env=env,text=True).strip()
        clean = not subprocess.check_output(['git','status','--porcelain'],cwd=source,env=env)
        info = subprocess.check_output([str(args.go_tool),'version','-m',str(args.go)],env=env,text=True)
        report['goBuildInfo']=info
        check('exact_clean_commit_binary_and_native_pins',head==args.commit and clean and 'vcs.revision='+args.commit in info and 'vcs.modified=false' in info and all(sha(getattr(args,name))==digest for name,digest in PINS.items()))
        client = start()
        catalog_source = client.request('/v1/core-profile-sources')['sources'][0]
        inspection = client.request('/v1/core-profile-inspection-consents',{'sourceId':catalog_source['id'],'expectedSourceRevision':catalog_source['revision'],'approveInspection':True,'ttlSeconds':600,'clientNonce':'finite-native-inspection'})['inspectionConsent']
        metadata = client.request('/v1/core-profiles?'+urllib.parse.urlencode({'sourceId':catalog_source['id'],'inspectionConsentId':inspection['id']}))
        profile = next(p for p in metadata['profiles'] if p['id']=='finite-fixture')
        check('actual_Core_catalog_inspection_no_model_or_source_mutation',profile['credentialReadiness']=='not_checked' and profile['bridgeTransport']=='supported' and not observations and sha(source_file)==initial_hash)
        if not args.rotation_only:
            channel,consent,consent_body,group_body,group = approve_group(client,catalog_source,profile,'complete')
            completed = wait_group(client,group['id'])
            report['completeGroup']=completed
            check('actual_three_stage_group_completed',completed['status']=='completed' and len(completed['requestIds'])==3)
            stages=[next(w for w in client.request('/v1/snapshot')['work'] if w['requestId']==identifier) for identifier in completed['requestIds']]
            report['completedStages']=stages
            plan=completed['coreProfileGroup']['plan']
            check('original_shared_plan_1_1_2_and_unspent_recovery_pool',plan['initialReserved']=={'requests':4,'steps':4,'outputTokens':2728} and plan['recoveryPool']=={'requests':2,'steps':2,'outputTokens':1364} and completed['coreProfileGroup']['budget']['recoveryReserved']=={'requests':0,'steps':0,'outputTokens':0} and not completed['coreProfileGroup']['budget']['recoveryReservations'])
            for index,work in enumerate(stages):
                stage=index+1
                pin=work['coreProfilePin']
                check('stage_%d_exact_pinned_Core_authority_and_limits'%stage,work['core']['coreSha256']==PINS['core'] and work['core']['nativeProcessId']>0 and pin['consentId']==consent['id'] and pin['limits']==plan['stageAllocations'][index]['limits'] and pin['expiresAtUnixMs']==plan['deadlineAtUnixMs'] and work['coreProfileGroupBinding']=={'version':1,'collaborationId':completed['id'],'stage':stage,'intentSha256':consent['groupIntentSha256']} and work['core']['contextAdmission']['contextPinSHA256']==work['contextPin']['sha256'] and work['core']['execution']['approval']=={'mode':'default','yolo':False})
                check('stage_%d_complete_prior_context_and_scoped_output'%stage,[p['requestId'] for p in work['contextPin']['sources']]==completed['requestIds'][:index] and all(p['reply']==stages[i]['reply'] for i,p in enumerate(work['contextPin']['sources'])) and (len(work.get('artifacts',[]))==1 and len(work.get('operations',[]))==1 if stage==3 and not args.text_only else not work.get('artifacts') and not work.get('operations')))
            check('completed_stages_all_have_native_stop_receipts',all(w.get('runtimeStopped') and w['runtimeStopReceipt']['nativeProcessId']==w['core']['nativeProcessId'] for w in stages))
            if not args.text_only:
                artifact=stages[2]['artifacts'][0]
                data=client.request('/v1/channels/'+channel['id']+'/artifacts/'+artifact['id'],raw=True)
                check('actual_final_artifact_bytes_hash',data==FILE_TEXT.encode() and hashlib.sha256(data).hexdigest()==artifact['sha256'])
            success_observations=[item for item in observations if item['groupId']==group['id']]
            expected_stages=[1,2,3] if args.text_only else [1,2,3,3]
            check('actual_provider_reservations_within_1_1_2_caps',[item['stage'] for item in success_observations]==expected_stages and [item['maxTokens'] for item in success_observations]==[682]*len(expected_stages))
            budget_files=list((state/'tasks').rglob('profile-bridge-budget-v1.json'))
            durable=[json.loads(path.read_text(encoding='utf-8')) for path in budget_files]
            report['coreTaskBudgetStates']=durable
            check('Core_task_home_durable_reservations_bounded',len(durable)==3 and sorted(v['requests'] for v in durable)==([1,1,1] if args.text_only else [1,1,2]) and sorted(v['reservedOutput'] for v in durable)==([682,682,682] if args.text_only else [682,682,1364]))
            stop()
            before=len(observations)
            client=start()
            replay=client.request('/v1/collaborations',group_body)
            check('restart_same_original_nonce_no_new_group_or_provider',replay['duplicate'] and replay['collaboration']==completed and len(observations)==before)
            receipt=client.request('/v1/core-profile-receipts?'+urllib.parse.urlencode({'kind':'group_execution','clientNonce':consent_body['clientNonce']}))
            check('restart_GET_group_receipt_retains_original_consent',receipt['found'] and receipt['consent']==consent and len(observations)==before)

        if not args.text_only and not args.rotation_only:
            for stage in (1,2):
                _,_,_,_,attempt=approve_group(client,catalog_source,profile,'deny%d'%stage)
                denied=wait_group(client,attempt['id'])
                works=[next(w for w in client.request('/v1/snapshot')['work'] if w['requestId']==identifier) for identifier in denied['requestIds']]
                check('actual_stage_%d_write_permission_denied_no_effect'%stage,denied['status']=='needs_owner' and len(works)==stage and all(not w.get('operations') and not w.get('artifacts') for w in works) and works[-1]['core']['nativeProcessId']>0 and works[-1]['runtimeStopped'])
        stopped_groups=[]
        for label in (('rotation',) if args.rotation_only else ('revoke','expiry','rotation')):
            _,c,cbody,gbody,g=approve_group(client,catalog_source,profile,label,True,20 if label=='expiry' else 300)
            until=time.monotonic()+45
            running=None
            while time.monotonic()<until:
                current=get_group(client,g['id'])
                if current['requestIds']:
                    identifier=current['requestIds'][0]
                    running=next(w for w in client.request('/v1/snapshot')['work'] if w['requestId']==identifier)
                    if running['status']=='running' and running.get('core',{}).get('nativeProcessId',0)>0 and 'FINITE_GROUP_HELD' in running.get('reply',''):
                        break
                time.sleep(.1)
            check(label+'_genuine_live_Core_before_authority_change',running is not None and running['status']=='running' and 'FINITE_GROUP_HELD' in running.get('reply',''))
            handle=CoreHandle(running['core']['nativeProcessId'],args.core)
            handles.append(handle)
            if label=='revoke':
                client.request('/v1/core-profile-consents/control',{'consentId':c['id'],'expectedRevision':c['revision'],'action':'revoke','clientNonce':'finite-native-revoke'})
            elif label=='rotation':
                write_profile('authored-finite-group-rotated-credential')
            work=client.wait(running['requestId'],lambda w:w['status'] in ('suspended','cancelled') and w.get('runtimeStopped') is True,timeout=30)
            current=get_group(client,g['id'])
            until=time.monotonic()+8
            while current['status']=='running' and time.monotonic()<until:
                time.sleep(.1)
                current=get_group(client,g['id'])
            closed = current['status']=='needs_owner' and current.get('reason')=='stage_budget_expired' and bool(current.get('budgetClosedAt')) if label=='expiry' else current['status']=='suspended'
            check(label+'_authority_stops_same_native_Core_and_original_pin',handle.stopped() and work['coreProfilePin']==running['coreProfilePin'] and closed and len(current['requestIds'])==1 and current['coreProfileGroup']['budget']==g['coreProfileGroup']['budget'] and not work.get('operations') and not work.get('artifacts'))
            before=len(observations)
            duplicate=client.request('/v1/collaborations',gbody)
            check(label+'_replay_retains_same_group_without_regrant',duplicate['duplicate'] and duplicate['collaboration']['id']==g['id'] and len(observations)==before)
            stopped_groups.append({'reason':label,'group':current,'work':work})
        report['stoppedGroups']=stopped_groups
        release.set()
        before=len(observations)
        time.sleep(.3)
        check('late_model_release_no_new_stage_or_provider',len(observations)==before and all(get_group(client,item['group']['id'])['status']==item['group']['status'] and get_group(client,item['group']['id'])['requestIds']==item['group']['requestIds'] for item in stopped_groups))
        public=json.dumps(client.request('/v1/snapshot'))+(state/'transactions.jsonl').read_text(encoding='utf-8')
        check('no_authored_credential_or_private_root_in_public_journal',FIXTURE_SECRET not in public and str(home) not in public)
        check('model_contract_all_admissions_bounded',not model_failures and all(item['credentialMatched'] and item['admitted'] and item['inputBytes']<=64000 for item in observations))
        check('source_hashes_unchanged_and_head_clean',all(sha(source/name)==digest for name,digest in report['dependencyHashes'].items()) and not subprocess.check_output(['git','status','--porcelain'],cwd=source,env=env) and subprocess.check_output(['git','rev-parse','HEAD'],cwd=source,env=env,text=True).strip()==args.commit)
        report['status']='PASS'
    except BaseException as error:
        report['status']='FAIL'
        report['failure']={'type':type(error).__name__,'label':str(error)}
        (out/'failure.txt').write_text(traceback.format_exc(),encoding='utf-8')
    finally:
        release.set()
        stop()
        for handle in handles:
            handle.close()
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)
        for log in logs:
            log.close()
        report['observations']=observations
        report['modelFailures']=model_failures
        report['cleanup']={'ownedServiceClosed':service is None,'ownedProviderThreadClosed':not server_thread.is_alive()}
        report['elapsedSeconds']=round(time.monotonic()-started,3)
        (out/'results.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'status':report['status'],'pass':sum(c['status']=='PASS' for c in checks),'fail':sum(c['status']=='FAIL' for c in checks),'failure':report.get('failure'),'cleanup':report['cleanup'],'results':str(out/'results.json')}))
    return 0 if report['status']=='PASS' else 1


if __name__=='__main__':
    raise SystemExit(main())
