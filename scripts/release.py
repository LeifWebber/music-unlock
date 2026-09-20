#!/usr/bin/env python3
"""Build standalone binaries and archives without publishing them."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ("darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64",
           "windows-amd64", "windows-arm64")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="dev")
    parser.add_argument("--target", choices=TARGETS, action="append")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.+-]*", args.version):
        parser.error("version must contain only letters, digits, dots, + and -")
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    archives = []
    for target in args.target or TARGETS:
        goos, goarch = target.split("-")
        name = f"music-unlock-{args.version}-{target}"
        folder = dist / name
        folder.mkdir(exist_ok=True)
        binary = folder / ("unmus.exe" if goos == "windows" else "unmus")
        subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags",
                        f"-s -w -X main.version={args.version}", "-o", str(binary),
                        "./cmd/unmus"], cwd=ROOT, check=True,
                       env={**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"})
        files = [binary]
        for filename in ("README.md", "LICENSE", "THIRD_PARTY_NOTICES.md"):
            output = folder / filename
            output.write_bytes((ROOT / filename).read_bytes())
            files.append(output)
        for source in sorted((ROOT / "docs").glob("*.md")):
            output = folder / "docs" / source.name
            output.parent.mkdir(exist_ok=True)
            output.write_bytes(source.read_bytes())
            files.append(output)
        if goos == "windows":
            archive = dist / f"{name}.zip"
            with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as out:
                for path in files:
                    out.write(path, arcname=f"{name}/{path.relative_to(folder).as_posix()}")
        else:
            archive = dist / f"{name}.tar.gz"
            with tarfile.open(archive, "w:gz") as out:
                for path in files:
                    out.add(path, arcname=f"{name}/{path.relative_to(folder).as_posix()}")
        archives.append(archive)
        print(archive.name, flush=True)
    sums = [f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n" for path in archives]
    (dist / "SHA256SUMS").write_text("".join(sums), encoding="utf-8")
    for installer in ("install.sh", "install.ps1", "install.cmd"):
        shutil.copyfile(ROOT / "scripts" / installer, dist / installer)
    print("Prepared dist/SHA256SUMS and shell/PowerShell/CMD installers", flush=True)


if __name__ == "__main__":
    main()
