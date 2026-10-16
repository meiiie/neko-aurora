"""Compile the hash-verified archived Core source in an isolated local build directory."""
import argparse
import hashlib
import json
import os
import pathlib
import shutil
import subprocess

ROOT=pathlib.Path(__file__).resolve().parents[1]
def main():
    parser=argparse.ArgumentParser();parser.add_argument("source",type=pathlib.Path);parser.add_argument("--bun",type=pathlib.Path,default=pathlib.Path(r"C:\Users\Admin\AppData\Roaming\npm\node_modules\bun\bin\bun.exe"));args=parser.parse_args()
    source=args.source.resolve();manifest_path=source.parent/"core-source-manifest.json";raw=manifest_path.read_bytes()
    if hashlib.sha256(raw).hexdigest()!="e2720458d4b2a42951ca3df30568f8409326745912500762242c82ae7429000e":raise ValueError("Pinned source manifest mismatch")
    manifest=json.loads(raw);destination=ROOT/".build/core-windows-source";destination.mkdir(parents=True,exist_ok=True)
    for item in manifest["files"]:
        relative=pathlib.PurePosixPath(item["path"])
        if relative.is_absolute() or ".." in relative.parts:raise ValueError("Unsafe source path")
        original=source.joinpath(*relative.parts);content=original.read_bytes()
        if len(content)!=item["bytes"] or hashlib.sha256(content).hexdigest()!=item["sha256"]:raise ValueError("Source checksum mismatch")
        target=destination.joinpath(*relative.parts);target.parent.mkdir(parents=True,exist_ok=True)
        if target.exists() and target.read_bytes()!=content:raise ValueError("Refusing to overwrite changed build source")
        target.write_bytes(content)
    build_home=ROOT/".build/core-build-home";build_home.mkdir(parents=True,exist_ok=True)
    environment={key:os.environ[key] for key in ["SystemRoot","WINDIR","TEMP","TMP","PATH"] if key in os.environ}
    environment.update({"USERPROFILE":str(build_home),"BUN_INSTALL_CACHE_DIR":str(ROOT.parent/".tooling/bun-cache")})
    executable=ROOT/".build/core-windows/neko.exe";executable.parent.mkdir(parents=True,exist_ok=True)
    subprocess.run([str(args.bun),"install","--frozen-lockfile","--ignore-scripts"],cwd=destination,env=environment,check=True)
    command=[str(args.bun),"build","--compile","--target=bun-windows-x64","--no-compile-autoload-dotenv","--no-compile-autoload-bunfig","--define",'process.env.NODE_ENV="production"',"--windows-title=Neko Core","--windows-description=Neko Core ACP runtime","--windows-copyright=AGPL-3.0-only; The Wiii Lab","--outfile",str(executable),"bin/neko.ts"]
    subprocess.run(command,cwd=destination,env=environment,check=True)
    version=subprocess.check_output([str(executable),"--version"],cwd=build_home,env=environment,text=True).strip()
    if version!="neko-core 1.8.0":raise ValueError("Unexpected compiled Core version")
    for name in ["LICENSE","NOTICE"]:
        if (source/name).exists():shutil.copyfile(source/name,executable.parent/name)
    evidence={"sourceRevision":manifest["revision"],"sourceManifestSha256":hashlib.sha256(raw).hexdigest(),"bunVersion":subprocess.check_output([str(args.bun),"--version"],env=environment,text=True).strip(),"binarySha256":hashlib.sha256(executable.read_bytes()).hexdigest(),"version":version,"license":"AGPL-3.0-only","dependencyLifecycleScriptsExecuted":False,"providerCalled":False}
    (executable.parent/"build-manifest.json").write_text(json.dumps(evidence,indent=2),encoding="utf-8")
    print(json.dumps(evidence,indent=2))
if __name__=="__main__":main()
