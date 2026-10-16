"""Restore only the pinned Core executable; never execute archive restore scripts."""
import argparse
import hashlib
import io
import json
import pathlib
import tarfile
import zipfile

PARTS = [
    ("01", "e38047b6b41a526699e9ee09f6c2a5c92aab3de4894786f1c791eceaadc28c71"),
    ("02", "912376002e8a76f9146da258e56657ad9f93e34e9bcff5efbd1d31b234c8f6df"),
]
TAR_SHA = "05d288f44a9a278f838dbc8b81dd0871e99504bd4d358a7643dbdcb2b2c8da65"
CORE_SHA = "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("archives", type=pathlib.Path)
    parser.add_argument("destination", type=pathlib.Path)
    args = parser.parse_args()
    combined = bytearray()
    for number, expected in PARTS:
        matches = list(args.archives.rglob(f"NekoBot-core-3d5725f-linux-x64-runtime-part{number}.zip"))
        if len(matches) != 1:
            raise ValueError("Expected exactly one pinned runtime part")
        with zipfile.ZipFile(matches[0]) as archive:
            members = [m for m in archive.infolist() if m.filename.endswith(f".part0{number}")]
            if len(members) != 1 or members[0].file_size > 20_000_000:
                raise ValueError("Invalid runtime part")
            content = archive.read(members[0])
        if hashlib.sha256(content).hexdigest() != expected:
            raise ValueError("Runtime part checksum mismatch")
        combined.extend(content)
    if hashlib.sha256(combined).hexdigest() != TAR_SHA:
        raise ValueError("Runtime archive checksum mismatch")
    with tarfile.open(fileobj=io.BytesIO(combined), mode="r:gz") as archive:
        member = archive.getmember("package/core/dist/neko")
        if not member.isfile() or member.size != 88_224_968:
            raise ValueError("Invalid executable member")
        content = archive.extractfile(member).read()
    if hashlib.sha256(content).hexdigest() != CORE_SHA or content[:4] != b"\x7fELF":
        raise ValueError("Core executable checksum mismatch")
    args.destination.mkdir(parents=True, exist_ok=True)
    target = args.destination / "neko"
    if target.exists() and target.read_bytes() != content:
        raise ValueError("Refusing to overwrite a different runtime")
    target.write_bytes(content)
    target.chmod(0o755)
    print(json.dumps({"revision": "3d5725f222815e15b12a212e372cf17057a459c0", "sha256": CORE_SHA, "executable": str(target.resolve()), "license": "AGPL-3.0-only", "executed": False}))


if __name__ == "__main__":
    main()
