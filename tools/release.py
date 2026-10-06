#!/usr/bin/env python3
"""Validate versions, resolve CI tag metadata, and stage only publishable files."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
VERSION_RE = re.compile(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?')


def validate(version):
    match = VERSION_RE.fullmatch(version)
    if not match or (match[4] and any(p.isdigit() and len(p) > 1 and p[0] == '0' for p in match[4].split('.'))):
        raise ValueError('Version must be X.Y.Z or X.Y.Z-prerelease (no leading zeros or build metadata)')
    return version


def metadata(ref_type, ref_name, fallback):
    if ref_type == 'tag':
        if not ref_name.startswith('v'):
            raise ValueError('Release tags must start with v')
        version = validate(ref_name[1:])
    else:
        version = validate(fallback.strip())
    return {'version': version, 'prerelease': str('-' in version).lower(),
            'artifact': f'mod-list-discovery-{version}-linux-amd64'}


def package(version, root=ROOT):
    validate(version)
    dist = root / 'dist'
    binary = dist / 'mod-list-discovery-server'
    jar = dist / f'mod-list-discovery-{version}+mc26.2.jar'
    # Refuse accidental host-architecture builds and mismatched/stale versions.
    header = binary.read_bytes()[:20]
    if header[:6] != b'\x7fELF\x02\x01' or header[18:20] != b'\x3e\x00':
        raise ValueError('Expected an ELF64 little-endian amd64 server binary')
    reported = subprocess.check_output([str(binary.resolve()), 'version'], text=True).strip()
    if reported != version:
        raise ValueError(f'Server reports {reported}, expected {version}')
    with zipfile.ZipFile(jar) as archive:
        mod = json.loads(archive.read('fabric.mod.json'))
        if mod['version'] != f'{version}+mc26.2' or mod['id'] != 'mod-list-discovery':
            raise ValueError('JAR metadata does not match release version')
        if not any(n.startswith('META-INF/jars/gson-') for n in archive.namelist()):
            raise ValueError('Production JAR must include Gson')
    target = dist / 'release'
    # This directory contains generated packaging output only, never deployment data.
    if target.exists():
        shutil.rmtree(target)
    target.mkdir()
    files = [(binary, f'mod-list-discovery-server-{version}-linux-amd64'),
             (jar, jar.name), (root / 'config.example.yaml', 'config.example.yaml')]
    for source, name in files:
        shutil.copy2(source, target / name)
    checksums = ''.join(f'{hashlib.sha256((target / name).read_bytes()).hexdigest()}  {name}\n'
                        for _, name in sorted(files, key=lambda item: item[1]))
    (target / 'SHA256SUMS').write_text(checksums, encoding='utf-8')
    print(f'Packaged {version} in {target}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['metadata', 'validate', 'package'])
    parser.add_argument('version', nargs='?')
    args = parser.parse_args()
    if args.command == 'metadata':
        values = metadata(os.getenv('GITHUB_REF_TYPE', ''), os.getenv('GITHUB_REF_NAME', ''),
                          (ROOT / 'VERSION').read_text())
        output = ''.join(f'{key}={value}\n' for key, value in values.items())
        if os.getenv('GITHUB_OUTPUT'):
            with open(os.environ['GITHUB_OUTPUT'], 'a', encoding='utf-8') as stream:
                stream.write(output)
        print(output, end='')
    elif args.command == 'package':
        package(args.version or (ROOT / 'VERSION').read_text().strip())
    else:
        validate(args.version or (ROOT / 'VERSION').read_text().strip())


if __name__ == '__main__':
    main()
