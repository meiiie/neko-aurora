import argparse
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--commit', required=True)
parser.add_argument('--output', type=pathlib.Path, required=True)
args = parser.parse_args()
assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip() == args.commit
assert not subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip()
environment = {key: os.environ[key] for key in ['SystemRoot', 'SystemDrive', 'WINDIR', 'ProgramData', 'ProgramFiles', 'ProgramFiles(x86)', 'ProgramW6432', 'CommonProgramFiles', 'TEMP', 'TMP', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA', 'PATH'] if key in os.environ}
with tempfile.TemporaryDirectory(prefix='fonts-state-', dir=ROOT / '.build') as directory:
    service = subprocess.Popen([str(ROOT / '.build/neko-control.exe'), '--listen', '127.0.0.1:0', '--state', str(pathlib.Path(directory) / 'state')], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=environment)
    try:
        ready = service.stdout.readline().strip()
        assert ready.startswith('READY '), 'Owned Go font service did not start'
        command = [shutil.which('node'), str(ROOT / 'scripts/ui_fonts_smoke.mjs'), r'C:\Program Files\Google\Chrome\Application\chrome.exe', ready[6:]]
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, timeout=45, env=environment)
        assert result.returncode == 0, result.stderr
        report = json.loads(result.stdout)
        assert report['status'] == 'pass'
        report.update({'codeCommit': args.commit, 'workingTree': False, 'goBinarySha256': hashlib.sha256((ROOT / '.build/neko-control.exe').read_bytes()).hexdigest(), 'command': 'python scripts/test_fonts_local.py --commit ' + args.commit + ' --output ' + str(args.output)})
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'status': 'pass', 'codeCommit': args.commit, 'fontsLoaded': len(report['fonts']), 'dataFontBlocked': report['dataBlocked'], 'report': str(args.output)}))
    finally:
        service.kill()
        service.wait(timeout=5)
