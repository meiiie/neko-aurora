import hashlib
import json
import pathlib
import re
import stat
import zipfile

repo = pathlib.Path(__file__).resolve().parent.parent
reference = repo / 'reference'
archive = reference / 'archive'
guide = (reference / 'handoff' / 'HUONG-DAN-CAC-BAN-CU.txt').read_text(encoding='utf-8')
expected = dict(re.findall(r'(Neko Aurora[^\r\n]+\.zip)\s*\r?\n([0-9a-f]{64})', guide))

def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024*1024), b''):
            h.update(block)
    return h.hexdigest()

def extract(source, destination, only_zips=False):
    destination.mkdir(parents=True, exist_ok=True)
    root = destination.resolve()
    with zipfile.ZipFile(source) as z:
        bad = z.testzip()
        if bad:
            raise RuntimeError('CRC mismatch in '+source.name)
        if sum(e.file_size for e in z.infolist()) > 2*1024**3:
            raise RuntimeError('Unexpected expanded size')
        for entry in z.infolist():
            if only_zips and not entry.filename.lower().endswith('.zip'):
                continue
            relative = pathlib.PurePosixPath(entry.filename)
            if relative.is_absolute() or '..' in relative.parts or '\\' in entry.filename or ':' in entry.filename:
                raise RuntimeError('Unsafe archive path')
            if stat.S_ISLNK(entry.external_attr >> 16):
                raise RuntimeError('Archive symlink rejected')
            target = (destination / pathlib.Path(*relative.parts)).resolve()
            if not target.is_relative_to(root):
                raise RuntimeError('Archive escaped root')
            if entry.is_dir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            data = z.read(entry)
            if target.exists():
                if target.read_bytes() != data:
                    raise RuntimeError('Existing file differs; refusing overwrite: '+str(target))
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)

grok = archive / 'Grok-Bot-client-va-phan-tich.zip'
bundle = archive / 'Neko Aurora-tat-ca-ban-cu-20261011.zip'
grok_hash = sha(grok)
if grok_hash != 'd31f6a0342693803510de086fae71a619ae07d20da9ea965e32d061c26f832e5':
    raise RuntimeError('Grok SHA differs from user-supplied original')
extract(grok, reference / 'extracted' / 'grok-0.68.1')
extract(bundle, archive / 'original', only_zips=True)
originals = list((archive / 'original').rglob('*.zip'))
if len(originals) != 15 or len(expected) != 15:
    raise RuntimeError('Original ZIP inventory incomplete')
inventory = []
by_name = {}
for path in sorted(originals):
    actual = sha(path)
    if path.name not in expected or actual != expected[path.name]:
        raise RuntimeError('Original ZIP hash mismatch: '+path.name)
    with zipfile.ZipFile(path) as z:
        if z.testzip():
            raise RuntimeError('Original ZIP CRC mismatch: '+path.name)
    inventory.append(dict(name=path.name, sha256=actual, crc='pass', size=path.stat().st_size))
    by_name[path.name] = path
for name in ['Neko Aurora-phase2-app-c9dbbd7-source-part01-of02.zip', 'Neko Aurora-phase2-app-c9dbbd7-source-part02-of02.zip']:
    extract(by_name[name], reference / 'extracted' / 'app-c9dbbd7')
extract(by_name['Neko Aurora-core-3d5725f-source.zip'], reference / 'extracted' / 'core-3d5725f')
sources = []
for snapshot, component in [('app-c9dbbd7', 'app'), ('core-3d5725f', 'core')]:
    package = reference / 'extracted' / snapshot / 'package'
    manifest = package / (component+'-source-manifest.json')
    metadata = json.loads(manifest.read_text(encoding='utf-8'))
    for entry in metadata['files']:
        file = (package / component / entry['path']).resolve()
        if not file.is_relative_to((package / component).resolve()) or not file.is_file() or file.stat().st_size != entry['bytes'] or sha(file) != entry['sha256']:
            raise RuntimeError('Source manifest mismatch: '+component+'/'+entry['path'])
    sources.append(dict(component=component, revision=metadata['revision'], fileCount=len(metadata['files']), manifestSha256=sha(manifest), verification='pass'))
report = dict(grok=dict(name=grok.name, sha256=grok_hash, crc='pass', libraryFileId='libfile_b497b1a479688191a77e358e94ccfcf1'), bundle=dict(name=bundle.name, sha256=sha(bundle), crc='pass', libraryFileId='libfile_ff7a113ffed88191b72c9bedc340d406'), originals=inventory, sources=sources, executed=False, sourceIntegration='none_reference_only')
(reference / 'inventory.json').write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
print(json.dumps(dict(grokSHA256=grok_hash, originalZIPs=len(inventory), allOriginalHashes='pass', allCRC='pass', executed=False)))
