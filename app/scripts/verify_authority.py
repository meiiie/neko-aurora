import json, os, pathlib, subprocess, sys, time

root = pathlib.Path(__file__).resolve().parents[1]
output = root / '.build' / ('authority-' + sys.argv[1])
output.mkdir(parents=True, exist_ok=True)
temporary = output / 'temp'
temporary.mkdir(exist_ok=True)
tooling = next(parent / '.tooling' for parent in root.parents if (parent / '.tooling/go/bin/go.exe').is_file())
env = dict(os.environ, TEMP=str(temporary), TMP=str(temporary), CGO_ENABLED='0', GOPROXY='off', GOSUMDB='off',
           GOCACHE=str(tooling / 'gocache'), GOPATH=str(tooling / 'gopath'))
go = tooling / 'go/bin/go.exe'
commands = [('go-tests', [str(go),'test','-json','./...','-count=1']), ('go-vet',[str(go),'vet','./...']),
            ('go-build',[str(go),'build','-o',str(output/'neko-aurora.exe'),'./cmd/neko-aurora'])]
report = {'commit': subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip(),
          'dirty': bool(subprocess.check_output(['git','status','--porcelain'],cwd=root)), 'commands': []}
for label, command in commands:
    started=time.time()
    result=subprocess.run(command,cwd=root/'native',env=env,capture_output=True)
    (output/(label+'.log')).write_bytes(result.stdout+result.stderr)
    entry={'name':label,'command':command,'exitCode':result.returncode,'seconds':round(time.time()-started,2)}
    if label=='go-tests':
        events=[]
        for line in result.stdout.splitlines():
            try: events.append(json.loads(line))
            except ValueError: pass
        entry['passTopLevel']=sum(x.get('Action')=='pass' and 'Test' in x and '/' not in x['Test'] for x in events)
        entry['passSubtests']=sum(x.get('Action')=='pass' and '/' in x.get('Test','') for x in events)
        entry['failedTests']=[x['Test'] for x in events if x.get('Action')=='fail' and 'Test' in x]
    report['commands'].append(entry)
(output/'validation.json').write_text(json.dumps(report,indent=2),encoding='utf-8')
print(json.dumps(report,ensure_ascii=True))
sys.exit(any(x['exitCode'] for x in report['commands']))
