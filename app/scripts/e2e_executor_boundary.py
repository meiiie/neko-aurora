import argparse
import copy
import hashlib
import json
import os
import pathlib
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument('--host', required=True)
parser.add_argument('--core', required=True)
parser.add_argument('--supervisor', required=True)
parser.add_argument('--scratch', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
host, core, supervisor = [pathlib.Path(x).resolve() for x in (args.host,args.core,args.supervisor)]
scratch = pathlib.Path(args.scratch).resolve()
scratch.mkdir(parents=True, exist_ok=True)
state = pathlib.Path(tempfile.mkdtemp(prefix='executor-boundary-',dir=scratch))
(state/'tasks').mkdir()
env = {k:v for k,v in os.environ.items() if k.upper() in {'SYSTEMROOT','WINDIR','TEMP','TMP','USERPROFILE'}}
results=[]
def gate(name, condition):
    results.append({'name':name,'status':'pass' if condition else 'fail'})
    if not condition:
        raise AssertionError(name)
def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()
probe=subprocess.run([str(host),'--executor-probe','--runtime',str(core),'--supervisor',str(supervisor),'--install-state-root',str(state)],env=env,capture_output=True,timeout=15)
gate('trusted_native_probe_exit',probe.returncode==0)
descriptor=json.loads(probe.stdout)
root_string=str(state).removeprefix('\\\\?\\').replace('\\','/').lower().rstrip('/') if os.name=='nt' else str(state).rstrip('/')
expected={'hostSha256':sha(host),'coreSha256':sha(core),'rootSha256':hashlib.sha256(root_string.encode()).hexdigest(),'capabilitiesSha256':hashlib.sha256(b'["executor_lease_v1","native_pid_session_stop_v1"]').hexdigest()}
gate('probe_actual_hashes_and_configured_state_scope',descriptor==expected)
pin={'version':1,'executorId':'executor-'+'a'*32,'generation':1,**descriptor}
now=int(time.time()*1000)
lease={'version':1,'id':'executor-lease-'+'b'*32,'executorId':pin['executorId'],'generation':1,'bootId':'executor-boot-'+'c'*32,'requestId':'req-'+'d'*32,'attemptId':'attempt-'+'e'*32,'workEpoch':1,'authoritySha256':'f'*64,'fence':1,'issuedAtMs':now,'expiresAtMs':now+600000}
spec={'requestId':lease['requestId'],'botId':'bot-meiiie','text':'Never send this text to Core or a provider.','executorPin':pin,'attemptLease':lease}
command=[str(host),'--runtime',str(core),'--supervisor',str(supervisor),'--state-root',str(state/'tasks'),'--fixture-endpoint','http://127.0.0.1:9/v1','--install-state-root',str(state)]
for name,mutate in [
 ('wrong_host_digest',lambda s:s['executorPin'].update(hostSha256='0'*64)),
 ('wrong_state_digest',lambda s:s['executorPin'].update(rootSha256=hashlib.sha256(str(state/'tasks').encode()).hexdigest())),
 ('expired_lease',lambda s:s['attemptLease'].update(issuedAtMs=1,expiresAtMs=2)),
 ('changed_generation',lambda s:s['attemptLease'].update(generation=2)),
 ('unknown_lease_field',lambda s:s['attemptLease'].update(unknown=1)),
 ('missing_fence',lambda s:s['attemptLease'].pop('fence')),
 ('unknown_install_version',lambda s:s['executorPin'].update(version=2)),
]:
    changed=copy.deepcopy(spec);mutate(changed)
    p=subprocess.run(command,input=json.dumps(changed,separators=(',',':')).encode()+b'\n',env=env,capture_output=True,timeout=15)
    gate(name+'_refused_before_task_directory',p.returncode!=0 and not (state/'tasks'/lease['requestId']).exists() and not p.stdout)
unconfigured=subprocess.run(command[:-2],input=json.dumps(spec).encode()+b'\n',env=env,capture_output=True,timeout=15)
gate('default_unconfigured_denied_before_directory',unconfigured.returncode!=0 and b'executor_unconfigured' in unconfigured.stderr and not (state/'tasks'/lease['requestId']).exists())
# Positive static-wire proof is synthetic; no registry issued this fixture lease.
# The only successful launch gate is explicitly rejected before any Core spawn.
p=subprocess.Popen(command,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env)
p.stdin.write(json.dumps(spec,separators=(',',':')).encode()+b'\n');p.stdin.flush()
first=json.loads(p.stdout.readline())
gate('validated_companion_launch_gate_before_core',first['sequence']==1 and first['kind']=='launch_ready' and first['data']['attemptId']==lease['attemptId'])
p.stdin.write(b'{"sequence":1,"ok":false,"result":{}}\n');p.stdin.flush()
remaining,stderr=p.communicate(timeout=15)
stop=json.loads(remaining)
proof={'version':1,'executorId':lease['executorId'],'generation':lease['generation'],'bootId':lease['bootId'],'leaseId':lease['id'],'requestId':lease['requestId'],'attemptId':lease['attemptId'],'workEpoch':lease['workEpoch'],'authoritySha256':lease['authoritySha256'],'fence':lease['fence']}
gate('gate_rejection_exact_never_launched_receipt',p.returncode!=0 and stop=={'sequence':2,'kind':'runtime_stopped','data':{'version':2,'kind':'never_launched','nativeProcessId':0,'sessionId':'','executorProof':proof}} and not (state/'tasks'/lease['requestId']).exists())
report={'scope':'Actual compiled Rust probe and pre-Core rejection; synthetic companion wire, no live registry lease, no Core launch/provider/network/GUI/pairing configuration.','state':str(state),'hostSha256':sha(host),'coreSha256':sha(core),'supervisorSha256':sha(supervisor),'results':results,'pass':len(results),'fail':0}
pathlib.Path(args.output).write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
print(f'PASS {len(results)} Rust pre-Core boundary gates; no Core launched')
