import hashlib,json,os,pathlib,subprocess,sys,time
source=pathlib.Path('E:/CodexScratch/group-core-profile-20261013/tool-source')
output=pathlib.Path('E:/CodexScratch/group-core-profile-20261013/tool-validation')/sys.argv[1]
output.mkdir(parents=True,exist_ok=False)
for name in ('home','appdata','localappdata','temp','gotmp','gomodcache'):(output/name).mkdir()
env={key:os.environ[key] for key in ('SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS') if key in os.environ}
env.update(USERPROFILE=str(output/'home'),HOME=str(output/'home'),HOMEDRIVE='E:',HOMEPATH=str(output/'home')[2:],APPDATA=str(output/'appdata'),LOCALAPPDATA=str(output/'localappdata'),TEMP=str(output/'temp'),TMP=str(output/'temp'),GOTMPDIR=str(output/'gotmp'),GOMODCACHE=str(output/'gomodcache'),GOCACHE='E:/CodexScratch/core-auth-bridge-20261012/root/gocache',GOENV='off',GOWORK='off',GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local',CGO_ENABLED='0',GIT_CONFIG_NOSYSTEM='1',GIT_CONFIG_GLOBAL='NUL',GIT_OPTIONAL_LOCKS='0')
go='C:/Users/Admin/Documents/Codex/2026-10-11/task/.tooling/go/bin/go.exe'
args=[go,'test','-json','-count=1']
if len(sys.argv)>4 and sys.argv[4]=='fixture':args.extend(['-tags','neko_native_fixture'])
args.extend(['-run',sys.argv[2] if len(sys.argv)>2 else '^TestProfileToolPermission',sys.argv[3] if len(sys.argv)>3 else './internal/ledger'])
files=[path for folder in ('ledger','executors','runtime') for path in (source/'native/internal'/folder).glob('*.go')]
hashes={str(path.relative_to(source)).replace('\\','/'):hashlib.sha256(path.read_bytes()).hexdigest() for path in files}
started=time.time()
run=subprocess.run(args,cwd=source/'native',env=env,capture_output=True,timeout=180)
(output/'go.stdout.jsonl').write_bytes(run.stdout);(output/'go.stderr.txt').write_bytes(run.stderr)
events=[json.loads(line) for line in run.stdout.decode('utf-8').splitlines() if line.startswith('{')]
checks=[{'name':event['Test'],'status':event['Action']} for event in events if event.get('Test') and event.get('Action') in ('pass','fail','skip')]
receipt=dict(baseCommit='c0786318f77b538b6cdf89ef7774733760a88c8a',sourceHashes=hashes,command=args,cwd=str(source/'native'),environmentNames=sorted(env),ownedPaths={key:env[key] for key in ('HOME','USERPROFILE','APPDATA','LOCALAPPDATA','TEMP','TMP','GOTMPDIR','GOMODCACHE','GOCACHE')},exitCode=run.returncode,elapsedSeconds=round(time.time()-started,3),checks=checks,stdoutSHA256=hashlib.sha256(run.stdout).hexdigest(),stderrSHA256=hashlib.sha256(run.stderr).hexdigest(),sourceUnchanged=all(hashlib.sha256((source/name).read_bytes()).hexdigest()==digest for name,digest in hashes.items()))
(output/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'output':str(output),'exitCode':run.returncode,'checks':checks,'sourceUnchanged':receipt['sourceUnchanged']}))
