import hashlib,json,os,pathlib,shutil,subprocess,sys,time
source=pathlib.Path('E:/CodexScratch/group-core-profile-20261013/tool-source')
output=pathlib.Path('E:/CodexScratch/group-core-profile-20261013/tool-validation')/sys.argv[1]
output.mkdir(parents=True,exist_ok=False)
for name in ('home','temp','cargo-home','target'):(output/name).mkdir()
shutil.copytree('C:/Users/Admin/.cargo/registry',output/'cargo-home/registry')
rust=pathlib.Path('C:/Users/Admin/.rustup/toolchains/1.97.1-x86_64-pc-windows-msvc/bin')
vc=pathlib.Path('C:/Program Files/Microsoft Visual Studio/2022/Preview/VC/Tools/MSVC/14.43.34618')
sdk=pathlib.Path('C:/Program Files (x86)/Windows Kits/10')
version='10.0.22621.0'
env={key:os.environ[key] for key in ('SystemRoot','WINDIR','COMSPEC','PATHEXT','PROCESSOR_ARCHITECTURE','NUMBER_OF_PROCESSORS') if key in os.environ}
env.update(HOME=str(output/'home'),USERPROFILE=str(output/'home'),TEMP=str(output/'temp'),TMP=str(output/'temp'),CARGO_HOME=str(output/'cargo-home'),CARGO_TARGET_DIR=str(output/'target'),CARGO_NET_OFFLINE='true',CARGO_INCREMENTAL='0',RUSTC=str(rust/'rustc.exe'),RUSTDOC=str(rust/'rustdoc.exe'),PATH=';'.join(map(str,(rust,vc/'bin/Hostx64/x64',sdk/('bin/'+version+'/x64'),pathlib.Path('C:/Windows/System32')))),LIB=';'.join(map(str,(vc/'lib/x64',sdk/('Lib/'+version+'/ucrt/x64'),sdk/('Lib/'+version+'/um/x64')))),INCLUDE=';'.join(map(str,(vc/'include',sdk/('Include/'+version+'/ucrt'),sdk/('Include/'+version+'/shared'),sdk/('Include/'+version+'/um')))),GIT_CONFIG_NOSYSTEM='1',GIT_CONFIG_GLOBAL='NUL',GIT_OPTIONAL_LOCKS='0')
command=[str(rust/'cargo.exe'),'test','--offline','--locked','--bin','neko-host','profile_permissions::tests','--','--nocapture']
files=list((source/'host/src').glob('*.rs'))+[source/'host/Cargo.toml',source/'host/Cargo.lock']
hashes={str(path.relative_to(source)).replace('\\','/'):hashlib.sha256(path.read_bytes()).hexdigest() for path in files}
started=time.time()
run=subprocess.run(command,cwd=source/'host',env=env,capture_output=True,timeout=180)
(output/'rust.stdout.txt').write_bytes(run.stdout);(output/'rust.stderr.txt').write_bytes(run.stderr)
receipt={'baseCommit':'c0786318f77b538b6cdf89ef7774733760a88c8a','command':command,'cwd':str(source/'host'),'environmentNames':sorted(env),'ownedPaths':{key:env[key] for key in ('HOME','USERPROFILE','TEMP','TMP','CARGO_HOME','CARGO_TARGET_DIR')},'toolchain':subprocess.check_output([str(rust/'rustc.exe'),'--version'],env=env,text=True).strip(),'offlineSourceCache':'C:/Users/Admin/.cargo/registry (copied only registry; no credentials.toml)','sourceHashes':hashes,'exitCode':run.returncode,'elapsedSeconds':round(time.time()-started,3),'stdoutSHA256':hashlib.sha256(run.stdout).hexdigest(),'stderrSHA256':hashlib.sha256(run.stderr).hexdigest(),'sourceUnchanged':all(hashlib.sha256((source/name).read_bytes()).hexdigest()==digest for name,digest in hashes.items())}
(output/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'output':str(output),'exitCode':run.returncode,'sourceUnchanged':receipt['sourceUnchanged'],'stdout':run.stdout.decode('utf-8'), 'stderrTail':run.stderr.decode('utf-8').splitlines()[-10:]}))
