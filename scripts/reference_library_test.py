"""Regression tests for offline reference-library releases (no external fixtures)."""
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import tarfile
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location('reference_library', Path(__file__).with_name('reference_library.py'))
ref = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ref)


class ReferenceLibraryTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.project = self.root / 'project'
        self.project.mkdir()
        self.write(self.project / 'go.mod', 'module example.com/project\n\ngo 1.23\n\nrequire example.com/Some/module v1.2.3\n')
        self.write(self.project / 'tracked.txt', 'committed source')
        self.write(self.project / 'package.json', json.dumps({'dependencies': {'fixture': '^2.0.0'}}))
        self.write(self.project / 'static/vendor/datastar-4.5.6.js', '// vendored runtime')
        self.write(self.project / 'static/vendor/README.md', 'Version: v4.5.6')
        self.git('init', '-q')
        self.git('add', '.')
        self.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.com', 'commit', '-qm', 'fixture')
        self.head = self.git('rev-parse', 'HEAD').strip()
        self.write(self.project / 'tracked.txt', 'dirty source')
        self.write(self.project / 'untracked.txt', 'untracked secret')
        package = self.project / 'node_modules/fixture'
        self.write(package / 'package.json', json.dumps({'name': 'fixture', 'version': '2.3.4', 'license': 'MIT', 'repository': {'url': 'git+https://github.com/example/fixture.git'}}))
        self.write(package / 'index.js', 'export const fixture = true;')
        self.cache = self.root / 'go-cache'
        self.write(self.cache / 'example.com/!some/module@v1.2.3/module.go', 'package module')
        self.library = self.root / 'library'
        self.base = self.library / 'releases/base'
        self.write(self.base / 'references/upstream/source.txt', 'upstream reference')
        self.write(self.base / 'bin/flid-ref', '#!/bin/sh\nexit 0\n')
        (self.base / 'bin/flid-ref').chmod(0o755)
        self.write(self.base / 'skills/reference/SKILL.md', 'reference skill')
        self.write(self.base / 'knowledge.lock.json', '{"upstream":"unaltered"}\n')
        self.write(self.base / 'CATALOG.md', '# Original catalog\n')
        self.upstream = {'id': 'upstream', 'title': 'Upstream', 'summary': 'Unchanged', 'path': str(self.base / 'references/upstream'), 'generated': {'revision': 'upstream-commit', 'file_count': 1, 'fetched_at': 'upstream-time'}, 'license': {'status': 'approved'}, 'acquisition': {'method': 'git'}, 'url': 'https://example.com/upstream', 'use_when': ['existing'], 'suggested_terms': ['existing']}
        self.write(self.base / 'catalog.json', json.dumps({'schema_version': 1, 'root': str(self.base), 'taxonomy': {'topics': {}, 'capabilities': {}}, 'sources': [self.upstream]}))
        self.write(self.base / 'release.json', json.dumps({'schema_version': 1, 'release_id': 'base', 'lock_sha256': ref.sha256(self.base / 'knowledge.lock.json')}))
        (self.library / 'current').symlink_to('releases/base', target_is_directory=True)
        common = {'title': 'Fixture', 'topics': ['testing'], 'capabilities': ['testing'], 'technologies': ['go'], 'terms': ['lookup'], 'owner': 'tracked.txt'}
        self.overlay = {'schema_version': 1, 'taxonomy': {'topics': {'testing': {}}, 'capabilities': {'testing': {}}}, 'sources': [
            dict(common, id='npm-fixture', kind='npm', package='fixture'),
            dict(common, id='go-fixture', kind='go', module='example.com/Some/module', url='https://example.com/go'),
            dict(common, id='datastar', kind='vendored', directory='static/vendor', pattern='datastar-*.js', companion_files=['README.md'], url='https://example.com/datastar'),
            dict(common, id='project', kind='project', url='https://example.com/project'),
        ], 'enhancements': {'upstream': {'use_when': ['existing', 'new guidance'], 'terms': ['new term']}}}

    def tearDown(self):
        # Installed releases are immutable, so temporary fixture teardown restores
        # directory writability without following inherited snapshot symlinks.
        for directory, _, _ in os.walk(self.root, followlinks=False):
            Path(directory).chmod(0o755)
        self.tmp.cleanup()

    def write(self, path, text):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.project), *args], text=True)

    def build(self, name='candidate', overlay=None, **kwargs):
        target = self.root / name
        report = ref.build(self.project, self.library, overlay or self.overlay, target, go_cache=self.cache, **kwargs)
        return target, report

    def test_versions_provenance_and_committed_project_snapshot(self):
        candidate, report = self.build()
        catalog = json.loads((candidate / 'catalog.json').read_text())
        lock = json.loads((candidate / 'local-references.lock.json').read_text())['sources']
        self.assertEqual(lock['npm-fixture']['version'], '2.3.4')
        self.assertEqual(lock['go-fixture']['version'], 'v1.2.3')
        self.assertEqual(lock['datastar']['version'], '4.5.6')
        self.assertEqual(lock['project']['version'], self.head)
        self.assertEqual((candidate / 'references/project/tracked.txt').read_text(), 'committed source')
        self.assertFalse((candidate / 'references/project/untracked.txt').exists())
        self.assertFalse((candidate / 'references/npm-fixture').is_symlink())
        self.assertEqual((candidate / 'knowledge.lock.json').read_bytes(), (self.base / 'knowledge.lock.json').read_bytes())
        entries = {entry['id']: entry for entry in catalog['sources']}
        for identifier, provenance in lock.items():
            self.assertGreater(entries[identifier]['generated']['file_count'], 0)
            self.assertNotIn('fetched_at', entries[identifier]['generated'])
            self.assertEqual(ref.content_info(candidate / 'references' / identifier), {key: provenance[key] for key in ref.content_info(candidate / 'references' / identifier)})
        self.assertEqual(entries['npm-fixture']['license']['name'], 'MIT')
        self.assertEqual(entries['npm-fixture']['url'], 'https://github.com/example/fixture.git')
        for field in ['generated', 'license', 'acquisition', 'url']:
            self.assertEqual(entries['upstream'][field], self.upstream[field])
        self.assertEqual(report['sources_added'], 4)

    def test_project_lookup_guidance_survives_install_and_refresh(self):
        definition = self.overlay['sources'][-1]
        definition['summary'] = 'Project source with a merged-PR guide.'
        definition['use_when'] = ['Start at docs/reference.md for recovery and chat changes.']
        ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        catalog = ref.read_json(self.library / 'current/catalog.json')
        entry = next(item for item in catalog['sources'] if item['id'] == 'project')
        self.assertEqual(entry['summary'], definition['summary'])
        self.assertEqual(entry['use_when'], definition['use_when'])

    def test_repeat_build_and_reapply_on_updated_base_are_idempotent(self):
        first, report1 = self.build()
        second, report2 = self.build('second')
        self.assertEqual(report1['release_id'], report2['release_id'])
        self.assertEqual((first / 'catalog.json').read_bytes(), (second / 'catalog.json').read_bytes())
        installed = ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        again = ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        self.assertEqual(installed['release_id'], again['release_id'])
        self.assertEqual(again['sources_added'], 0)
        current = (self.library / 'current').resolve()
        entries = json.loads((current / 'catalog.json').read_text())['sources']
        upstream = next(entry for entry in entries if entry['id'] == 'upstream')
        self.assertEqual(upstream['use_when'], ['existing', 'new guidance'])
        self.assertEqual(upstream['suggested_terms'], ['existing', 'new term'])
        self.assertTrue(self.base.exists())
        self.assertFalse(current.stat().st_mode & 0o222)
        self.assertFalse((current / 'references/npm-fixture/index.js').stat().st_mode & 0o222)

    def test_preserved_skill_changes_are_part_of_release_identity(self):
        _, first = self.build()
        self.write(self.base / 'skills/reference/SKILL.md', 'new reference skill')
        candidate, second = self.build('second')
        self.assertNotEqual(first['release_id'], second['release_id'])
        self.assertEqual((candidate / 'skills/reference/SKILL.md').read_text(), 'new reference skill')

    def test_existing_upstream_id_is_preserved(self):
        overlay = copy.deepcopy(self.overlay)
        overlay['sources'][0]['id'] = 'upstream'
        candidate, report = self.build(overlay=overlay)
        entries = json.loads((candidate / 'catalog.json').read_text())['sources']
        self.assertEqual(sum(entry['id'] == 'upstream' for entry in entries), 1)
        self.assertEqual(next(entry for entry in entries if entry['id'] == 'upstream')['generated'], self.upstream['generated'])
        self.assertEqual(report['sources_added'], 3)

    def test_failed_install_keeps_current_and_cleans_own_stage(self):
        original = os.readlink(self.library / 'current')
        real_replace = os.replace
        def fail_switch(source, target):
            if Path(target) == self.library / 'current':
                raise OSError('injected switch failure')
            return real_replace(source, target)
        with mock.patch.object(ref.os, 'replace', side_effect=fail_switch):
            with self.assertRaisesRegex(OSError, 'injected'):
                ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        self.assertEqual(os.readlink(self.library / 'current'), original)
        self.assertEqual(list((self.library / '.incoming').iterdir()), [])
        self.assertFalse(list(self.library.glob('.current-*')))

    def test_invalid_ids_and_external_current_are_rejected(self):
        for bad in ['../escape', '/absolute', '.', 'nested/id', 'UPPER', 'back\\slash']:
            with self.subTest(bad=bad):
                overlay = copy.deepcopy(self.overlay)
                overlay['sources'][0]['id'] = bad
                with self.assertRaises(ValueError):
                    self.build(overlay=overlay)
                self.assertFalse((self.root / 'candidate').exists())
        (self.library / 'current').unlink()
        (self.library / 'current').symlink_to(self.project)
        with self.assertRaises(ValueError):
            self.build()

    def test_local_symlink_escape_and_vendored_traversal_are_rejected(self):
        (self.project / 'node_modules/fixture/secret').symlink_to(self.project / 'untracked.txt')
        with self.assertRaises(ValueError):
            self.build()
        (self.project / 'node_modules/fixture/secret').unlink()
        overlay = copy.deepcopy(self.overlay)
        overlay['sources'][2]['companion_files'] = ['../../../untracked.txt']
        with self.assertRaises(ValueError):
            self.build(overlay=overlay)

    def test_atomic_switch_renames_a_writable_stage_after_validation(self):
        original_rename = Path.rename
        original_replace = os.replace
        events = []
        def rename_stage(path, target):
            self.assertTrue(path.stat().st_mode & 0o222)
            self.assertEqual(os.readlink(self.library / 'current'), 'releases/base')
            events.append('rename')
            return original_rename(path, target)
        def switch(source, target):
            final = Path(source).resolve()
            self.assertFalse(final.stat().st_mode & 0o222)
            self.assertTrue((final / 'release.json').exists())
            events.append('switch')
            return original_replace(source, target)
        with mock.patch.object(Path, 'rename', rename_stage), mock.patch.object(ref.os, 'replace', side_effect=switch):
            ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        self.assertEqual(events, ['rename', 'switch'])

    def test_existing_release_tampering_is_rejected_without_switching_current(self):
        report = ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        final = self.library / 'releases' / report['release_id']
        (self.library / 'current').unlink()
        (self.library / 'current').symlink_to('releases/base', target_is_directory=True)
        source = final / 'references/npm-fixture/index.js'
        source.chmod(0o644)
        source.write_text('tampered snapshot')
        with self.assertRaisesRegex(ValueError, 'snapshot differs'):
            ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        self.assertEqual(os.readlink(self.library / 'current'), 'releases/base')

    def test_git_archive_traversal_and_links_are_rejected(self):
        for name, member_type in [('../escape', tarfile.REGTYPE), ('link', tarfile.SYMTYPE)]:
            with self.subTest(name=name):
                data = io.BytesIO()
                with tarfile.open(fileobj=data, mode='w') as archive:
                    member = tarfile.TarInfo(name)
                    member.type = member_type
                    member.linkname = '/outside' if member_type == tarfile.SYMTYPE else ''
                    member.size = 0
                    archive.addfile(member)
                process = mock.Mock(stdout=io.BytesIO(data.getvalue()))
                process.wait.return_value = 0
                destination = self.root / 'unsafe-archive'
                with mock.patch.object(ref, 'command', return_value=self.head), mock.patch.object(ref.subprocess, 'Popen', return_value=process):
                    with self.assertRaises(ValueError):
                        ref.archive_head(self.project, destination)
                self.assertFalse((self.root / 'escape').exists())
                shutil.rmtree(destination)

    def test_missing_package_does_not_change_installation(self):
        shutil.rmtree(self.project / 'node_modules/fixture')
        with self.assertRaises((ValueError, FileNotFoundError)):
            ref.install(self.project, self.library, self.overlay, go_cache=self.cache)
        self.assertEqual(os.readlink(self.library / 'current'), 'releases/base')


if __name__ == '__main__':
    unittest.main()
