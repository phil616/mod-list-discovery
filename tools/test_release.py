import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

from release import metadata, package, validate


class ReleaseTests(unittest.TestCase):
    def test_tag_and_branch_versions(self):
        self.assertEqual(metadata('tag', 'v2.3.4', '0.1.0')['version'], '2.3.4')
        self.assertEqual(metadata('tag', 'v2.3.4-rc.1', '0.1.0')['prerelease'], 'true')
        self.assertEqual(metadata('branch', 'feature/example', '0.1.0\n')['version'], '0.1.0')
        for tag in ['v1.2', 'v01.2.3', 'v1.2.3-01', 'v1.2.3+other', 'v1.2.3\nx=y', 'v$(id)', '1.2.3']:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                metadata('tag', tag, '0.1.0')
        self.assertEqual(validate('1.2.3-beta.0'), '1.2.3-beta.0')

    def test_package_allowlist_and_version_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            dist = root / 'dist'
            dist.mkdir()
            (dist / 'data').mkdir()
            (dist / 'data/private.key').write_text('do not publish')
            (dist / 'old-sources.jar').write_text('do not publish')
            (root / 'config.example.yaml').write_text('storage: {}')
            (dist / 'mod-list-discovery-server').write_bytes(b'\x7fELF\x02\x01' + b'\0' * 12 + b'\x3e\x00')
            jar = dist / 'mod-list-discovery-2.3.4+mc26.2.jar'
            with zipfile.ZipFile(jar, 'w') as z:
                z.writestr('fabric.mod.json', json.dumps({'id': 'mod-list-discovery', 'version': '2.3.4+mc26.2'}))
                z.writestr('META-INF/jars/gson-test.jar', b'fixture')
            with patch('release.subprocess.check_output', return_value='2.3.4\n'):
                package('2.3.4', root)
            self.assertEqual({p.name for p in (dist / 'release').iterdir()},
                             {'mod-list-discovery-server-2.3.4-linux-amd64', jar.name, 'config.example.yaml', 'SHA256SUMS'})
            for line in (dist / 'release/SHA256SUMS').read_text().splitlines():
                checksum, name = line.split('  ')
                self.assertEqual(checksum, hashlib.sha256((dist / 'release' / name).read_bytes()).hexdigest())
            with patch('release.subprocess.check_output', return_value='0.1.0\n'), self.assertRaises(ValueError):
                package('2.3.4', root)

    def test_publication_is_draft_first_and_retry_safe(self):
        script = Path(__file__).with_name('publish-release.sh').resolve()
        for existing, fail_upload, expected_publish in [('missing', False, True), ('true', False, True),
                                                       ('false', False, False), ('missing', True, False)]:
            with self.subTest(existing=existing, fail_upload=fail_upload), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                assets = root / 'dist/release'
                assets.mkdir(parents=True)
                for name in ['mod-list-discovery-server-2.3.4-rc.1-linux-amd64',
                             'mod-list-discovery-2.3.4-rc.1+mc26.2.jar', 'config.example.yaml']:
                    (assets / name).write_text('fixture')
                (assets / 'SHA256SUMS').write_text(''.join(
                    f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in sorted(assets.iterdir())))
                (root / 'tools').mkdir()
                (root / 'tools/release.py').write_text(Path(__file__).with_name('release.py').read_text())
                gh = root / 'gh'
                gh.write_text('''#!/usr/bin/env python3
import os,sys
from pathlib import Path
with open('calls','a') as f: f.write(' '.join(sys.argv[1:])+'\\n')
if sys.argv[2]=='view':
 print(os.environ['EXISTING']); sys.exit(1 if os.environ['EXISTING']=='missing' else 0)
if sys.argv[2]=='upload' and os.environ['FAIL_UPLOAD']=='1': sys.exit(1)
''')
                gh.chmod(0o755)
                result = subprocess.run(['bash', str(script)], cwd=root, capture_output=True, text=True,
                                        env={**os.environ, 'PATH': str(root) + os.pathsep + os.environ['PATH'],
                                             'GH_REPO': 'test/test', 'GH_TOKEN': 'fixture', 'RELEASE_TAG': 'v2.3.4-rc.1',
                                             'RELEASE_VERSION': '2.3.4-rc.1', 'EXISTING': existing,
                                             'FAIL_UPLOAD': '1' if fail_upload else '0'})
                calls = (root / 'calls').read_text()
                self.assertEqual('release edit' in calls, expected_publish, result.stderr)
                self.assertEqual(result.returncode == 0, not fail_upload, result.stderr)
                if existing == 'false':
                    self.assertNotIn('release upload', calls)
                if existing == 'missing':
                    self.assertIn('--verify-tag --draft --generate-notes', calls)
                    self.assertLess(calls.index('release create'), calls.index('release upload'))
                if expected_publish:
                    self.assertIn('--prerelease --latest=false', calls)
                    self.assertLess(calls.index('release upload'), calls.index('release edit'))
