import hashlib,json,os,pathlib,subprocess,sys,time
source=pathlib.Path('E:/CodexScratch/group-core-profile-20261013/backend-source')
output=pathlib.Path('E:/CodexScratch/group-core-profile-20261013/backend-validation')/sys.argv[1]
output.mkdir(parents=True,exist_ok=False)
for name in ['home','appdata','localappdata','temp','gotmp','gomodcache']:(output/name).mkdir()
env={k:os.environ[k] for k in ['SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS'] if k in os.environ}
env.update(USERPROFILE=str(output/'home'),HOMEDRIVE='E:',HOMEPATH=str(output/'home')[2:],APPDATA=str(output/'appdata'),LOCALAPPDATA=str(output/'localappdata'),TEMP=str(output/'temp'),TMP=str(output/'temp'),GOTMPDIR=str(output/'gotmp'),GOMODCACHE=str(output/'gomodcache'),GOCACHE='E:/CodexScratch/core-auth-bridge-20261012/root/gocache',GOENV='off',GOWORK='off',GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local',CGO_ENABLED='0',GIT_CONFIG_NOSYSTEM='1',GIT_CONFIG_GLOBAL='NUL',GIT_OPTIONAL_LOCKS='0')
go='C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe'
args=[go,'test','-json','-count=1','-run',sys.argv[2],sys.argv[3] if len(sys.argv)>3 else './internal/ledger']
files=list((source/'native/internal/ledger').glob('*.go'))
hashes={str(f.relative_to(source)).replace('\\','/'):hashlib.sha256(f.read_bytes()).hexdigest() for f in files}
started=time.time()
try:
 run=subprocess.run(args,cwd=source/'native',env=env,capture_output=True,timeout=180)
 (output/'go.stdout.jsonl').write_bytes(run.stdout);(output/'go.stderr.txt').write_bytes(run.stderr)
 events=[json.loads(line) for line in run.stdout.decode('utf-8').splitlines() if line.startswith('{')]
 checks=[{'name':e['Test'],'status':e['Action']} for e in events if e.get('Test') and e.get('Action') in ['pass','fail','skip']]
 receipt=dict(baseCommit='7f80d55948d4e9847fdbe68a9557e29c88025014',sourceHashes=hashes,command=args,cwd=str(source/'native'),environmentNames=sorted(env),ownedPaths={k:env[k] for k in ['USERPROFILE','APPDATA','LOCALAPPDATA','TEMP','TMP','GOTMPDIR','GOMODCACHE','GOCACHE']},exitCode=run.returncode,elapsedSeconds=round(time.time()-started,3),checks=checks,stdoutSHA256=hashlib.sha256(run.stdout).hexdigest(),stderrSHA256=hashlib.sha256(run.stderr).hexdigest(),sourceUnchanged=all(hashlib.sha256((source/f).read_bytes()).hexdigest()==h for f,h in hashes.items()))
 (output/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf-8')
 print(json.dumps({'output':str(output),'exitCode':run.returncode,'checks':checks,'sourceUnchanged':receipt['sourceUnchanged']}))
except subprocess.TimeoutExpired as e:
 (output/'go.stdout.jsonl').write_bytes(e.stdout or b'');(output/'go.stderr.txt').write_bytes(e.stderr or b'')
 (output/'blocked.json').write_text(json.dumps(dict(reason='go_timeout',command=args,sourceHashes=hashes,elapsedSeconds=time.time()-started))+'\n');raise
