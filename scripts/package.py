#!/usr/bin/env python3
"""Build every published GitMake artifact from a clean source tree.

Packaging used to be manual: cross-compile by hand, zip by hand, write the
checksum file by hand. Nothing recorded what belonged in a release, so a stray
file could ship (v1.2.5 published an empty install_windows.go.tmp) and the
checksum manifest could be written with CRLF endings that `sha256sum -c`
refuses to read.

    python3 scripts/package.py --out dist/release

Produces the platform ZIPs, the source ZIP, the SHA-256 manifest, and the
self-publish folder that `gitmake` itself consumes to publish a release. It
never talks to GitHub; publishing stays a reviewed, human-approved GitMake run.
"""

import argparse
import hashlib
import os
import re
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Documentation shipped inside every platform package.
DOCS = [
    "QUICKSTART.txt", "STABILITY.md", "GITMAKE_FOR_LLM.md", "TEST_REPORT.md",
    "CLAUDE_MCP_SETUP.txt", "LICENSE", "RELEASE_NOTES.md", "README.md", "CHANGELOG.md",
]
EXAMPLES = ["examples/gitmake.folder.json", "examples/gitmake.json"]

# Extra top-level files that belong in the source package.
SOURCE_EXTRA = [
    ".gitignore", ".gitattributes", "INSTALL_GITMAKE.bat", "RUN_GITMAKE.bat",
    "build.bat", "build.ps1", "go.mod",
]
SOURCE_DIRS = ["cmd", "internal", "scripts", "examples", ".github"]

# name suffix, GOOS, GOARCH, binaries as (built name, name inside the ZIP)
PLATFORMS = [
    ("Windows_x64", "windows", "amd64", [("gitmake.exe", "gitmake.exe"), ("GitMake-Setup.exe", "GitMake-Setup.exe")]),
    ("Linux_x64", "linux", "amd64", [("gitmake", "gitmake")]),
    ("Linux_arm64", "linux", "arm64", [("gitmake", "gitmake")]),
    ("macOS_x64", "darwin", "amd64", [("gitmake", "gitmake")]),
    ("macOS_arm64", "darwin", "arm64", [("gitmake", "gitmake")]),
]

# Never ship build output or editor/OS debris in the source package.
SOURCE_SKIP_SUFFIXES = {".exe", ".test", ".tmp", ".zip"}

# A fixed timestamp keeps ZIP bytes reproducible across machines.
ZIP_TIMESTAMP = (1980, 1, 1, 0, 0, 0)


def detect_version() -> str:
    text = (ROOT / "internal" / "app" / "app.go").read_text(encoding="utf-8")
    match = re.search(r'const Version = "([^"]+)"', text)
    if not match:
        sys.exit("could not find `const Version` in internal/app/app.go")
    return match.group(1)


def run(cmd, env=None):
    merged = dict(os.environ)
    if env:
        merged.update(env)
    result = subprocess.run(cmd, cwd=ROOT, env=merged)
    if result.returncode != 0:
        sys.exit("command failed: %s" % " ".join(cmd))


def build_binaries(stage: Path):
    for suffix, goos, goarch, binaries in PLATFORMS:
        target = stage / ("%s_%s" % (goos, goarch))
        target.mkdir(parents=True, exist_ok=True)
        for built_name, _ in binaries:
            pkg = "./cmd/setup" if built_name.startswith("GitMake-Setup") else "./cmd/gitmake"
            print("building %s/%s %s" % (goos, goarch, built_name), flush=True)
            run(
                ["go", "build", "-trimpath", "-ldflags", "-s -w", "-o", str(target / built_name), pkg],
                env={"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"},
            )


def add(zf: zipfile.ZipFile, disk: Path, arc: str, executable: bool = False):
    info = zipfile.ZipInfo(arc, date_time=ZIP_TIMESTAMP)
    info.compress_type = zipfile.ZIP_DEFLATED
    info.external_attr = (0o755 if executable else 0o644) << 16
    zf.writestr(info, disk.read_bytes())


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_lf(path: Path, text: str):
    """Release metadata must use LF. Python's text mode translates newlines on
    Windows, and a CRLF manifest cannot be verified with `sha256sum -c`."""
    with path.open("w", encoding="utf-8", newline="") as handle:
        handle.write(text.replace("\r\n", "\n"))


def package_platforms(out: Path, stage: Path, version: str) -> list:
    names = []
    for suffix, goos, goarch, binaries in PLATFORMS:
        name = "GitMake_v%s_%s.zip" % (version, suffix)
        with zipfile.ZipFile(out / name, "w", zipfile.ZIP_DEFLATED) as zf:
            for doc in DOCS:
                add(zf, ROOT / doc, doc)
            for example in EXAMPLES:
                add(zf, ROOT / example, example)
            for built_name, arc_name in binaries:
                add(zf, stage / ("%s_%s" % (goos, goarch)) / built_name, arc_name, executable=True)
        print("packaged", name)
        names.append(name)
    return names


def package_source(out: Path, version: str) -> str:
    name = "GitMake_v%s_Source.zip" % version
    prefix = "gitmake-v%s" % version
    count = 0
    with zipfile.ZipFile(out / name, "w", zipfile.ZIP_DEFLATED) as zf:
        for doc in DOCS + SOURCE_EXTRA:
            add(zf, ROOT / doc, "%s/%s" % (prefix, doc))
            count += 1
        for directory in SOURCE_DIRS:
            base = ROOT / directory
            if not base.exists():
                continue
            for path in sorted(base.rglob("*")):
                if path.is_dir() or path.suffix in SOURCE_SKIP_SUFFIXES:
                    continue
                rel = path.relative_to(ROOT).as_posix()
                add(zf, path, "%s/%s" % (prefix, rel), executable=path.suffix in {".sh", ".py"})
                count += 1
    print("packaged %s (%d files)" % (name, count))
    return name


def write_self_publish(out: Path, stage: Path, version: str):
    shutil.copy2(stage / "windows_amd64" / "gitmake.exe", out / "gitmake.exe")
    for doc in ["RELEASE_NOTES.md", "STABILITY.md", "TEST_REPORT.md", "GITMAKE_FOR_LLM.md"]:
        shutil.copy2(ROOT / doc, out / doc)

    title = release_title(version)
    assets = ",\n      ".join(
        '"GitMake_v%s_%s"' % (version, part)
        for part in [
            "Windows_x64.zip", "Linux_x64.zip", "Linux_arm64.zip",
            "macOS_x64.zip", "macOS_arm64.zip", "Source.zip", "SHA256.txt",
        ]
    )
    config = """{
  "schema_version": 1,
  "repo": {
    "owner": "stickleetoto",
    "name": "GitMake",
    "description": "AI-safe GitHub publishing workflow for project folders and ZIPs.",
    "visibility": "public"
  },
  "source": {
    "zip": "GitMake_v%(v)s_Source.zip",
    "strip_root": true
  },
  "git": {
    "branch": "main",
    "initial_commit_message": "Initial commit",
    "commit_message": "Release GitMake v%(v)s"
  },
  "release": {
    "enabled": true,
    "tag": "v%(v)s",
    "title": %(title)s,
    "notes_file": "RELEASE_NOTES.md",
    "assets": [
      %(assets)s
    ],
    "on_existing": "error"
  }
}
""" % {"v": version, "title": title, "assets": assets}
    write_lf(out / "gitmake.json", config)

    write_lf(out / "PUBLISH.txt", """GitMake v%(v)s Self-Publish

Run gitmake.exe in this folder.
It updates stickleetoto/GitMake and creates Release v%(v)s.

Before publishing, review RELEASE_NOTES.md and GitMake_v%(v)s_SHA256.txt.

This package is built, not published. Publishing is still a reviewed GitMake run.
""" % {"v": version})


def release_title(version: str) -> str:
    """Reuse the release notes' own H1 so the tag title cannot drift from it."""
    first = (ROOT / "RELEASE_NOTES.md").read_text(encoding="utf-8").splitlines()[0]
    heading = first.lstrip("#").strip()
    if not heading:
        heading = "GitMake v%s" % version
    import json
    return json.dumps(heading, ensure_ascii=False)


def main():
    parser = argparse.ArgumentParser(description="Build all published GitMake artifacts.")
    parser.add_argument("--out", default="dist/release", help="output directory (default: dist/release)")
    parser.add_argument("--skip-build", action="store_true", help="reuse binaries already staged under <out>/.stage")
    args = parser.parse_args()

    version = detect_version()
    out = (ROOT / args.out).resolve() if not Path(args.out).is_absolute() else Path(args.out)
    stage = out / ".stage"

    out.mkdir(parents=True, exist_ok=True)
    for existing in out.iterdir():
        if existing == stage:
            continue
        shutil.rmtree(existing) if existing.is_dir() else existing.unlink()

    print("GitMake v%s -> %s" % (version, out))
    if not args.skip_build:
        if stage.exists():
            shutil.rmtree(stage)
        build_binaries(stage)

    names = package_platforms(out, stage, version)
    names.append(package_source(out, version))

    manifest = "\n".join("%s  %s" % (sha256(out / name), name) for name in names) + "\n"
    write_lf(out / ("GitMake_v%s_SHA256.txt" % version), manifest)
    print("wrote checksum manifest")

    write_self_publish(out, stage, version)
    shutil.rmtree(stage, ignore_errors=True)
    print("done: %s" % out)


if __name__ == "__main__":
    main()
