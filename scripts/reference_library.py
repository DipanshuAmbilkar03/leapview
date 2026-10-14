#!/usr/bin/env python3
"""Build an offline local overlay for an existing Flid reference-library release.

Dependency distributions, cached Go modules, vendored runtime files, and the
project's committed Git HEAD are copied independently. Existing upstream
references and knowledge.lock.json retain their acquisition metadata. This is
local maintenance; it neither fetches upstreams nor publishes a library release.
"""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import tarfile
import tempfile

ID = re.compile(r'[a-z0-9][a-z0-9_-]*\Z')
HASH_FILES = {'catalog.json': 'catalog_sha256', 'CATALOG.md': 'catalog_markdown_sha256',
              'knowledge.lock.json': 'lock_sha256', 'bin/flid-ref': 'tool_sha256',
              'local-references.lock.json': 'local_lock_sha256'}
KINDS = {'npm': 'installed-package', 'go': 'go-module-cache',
         'vendored': 'vendored-browser-runtime', 'project': 'tracked-source-archive'}


def read_json(path):
    return json.loads(path.read_text(encoding='utf-8'))


def dump(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')


def sha256(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def content_info(path):
    """Measure copied regular files, including their relative names in the hash."""
    if path.is_symlink() or not path.is_dir():
        raise ValueError(f'Expected an independent snapshot directory: {path}')
    digest = hashlib.sha256()
    size = count = 0
    for file in sorted(path.rglob('*')):
        if file.is_symlink():
            raise ValueError(f'Snapshot contains a symlink: {file}')
        if file.is_dir():
            continue
        if not file.is_file():
            raise ValueError(f'Snapshot contains a special file: {file}')
        digest.update(file.relative_to(path).as_posix().encode() + b'\0')
        with file.open('rb') as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(chunk)
                size += len(chunk)
        digest.update(b'\0')
        count += 1
    return {'file_count': count, 'size_bytes': size, 'local_content_sha256': digest.hexdigest()}


def identifier(value):
    if not isinstance(value, str) or not ID.fullmatch(value):
        raise ValueError(f'Unsafe source or release ID: {value!r}')
    return value


def relative(value):
    """Accept only normalized, portable relative paths."""
    if not isinstance(value, str) or '\\' in value or '\x00' in value:
        raise ValueError(f'Unsafe relative path: {value!r}')
    path = PurePosixPath(value)
    if not value or path.is_absolute() or any(part in ('', '.', '..') for part in value.split('/')):
        raise ValueError(f'Unsafe relative path: {value!r}')
    return path


def within(path, root):
    resolved = path.resolve(strict=True)
    if not resolved.is_relative_to(root.resolve(strict=True)):
        raise ValueError(f'Path escapes its source root: {path}')
    return resolved


def copy_local(source, target, root=None, ancestors=()):
    """Dereference only contained links, reject cycles and special files."""
    root = source.resolve(strict=True) if root is None else root
    actual = within(source, root)
    if actual in ancestors:
        raise ValueError(f'Symlink cycle in local source: {source}')
    if actual.is_dir():
        target.mkdir()
        for child in sorted(actual.iterdir()):
            copy_local(child, target / child.name, root, ancestors + (actual,))
    elif actual.is_file():
        shutil.copyfile(actual, target)
        target.chmod(0o755 if actual.stat().st_mode & 0o111 else 0o644)
    else:
        raise ValueError(f'Unsupported local file: {source}')


def command(args, project):
    # Explicitly disable Go proxy/toolchain downloads, even in user environments
    # that normally auto-install a requested toolchain.
    env = dict(os.environ, GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GOWORK='off')
    return subprocess.check_output(args, cwd=project, env=env, text=True).strip()


def validate_overlay(overlay):
    if overlay.get('schema_version') != 1:
        raise ValueError('Unsupported overlay schema_version')
    seen = set()
    for source in overlay['sources']:
        key = identifier(source['id'])
        if key in seen:
            raise ValueError(f'Duplicate overlay source ID: {key}')
        seen.add(key)
        if source['kind'] not in KINDS:
            raise ValueError(f'Unsupported local source kind: {source["kind"]}')
        for field in ('topics', 'capabilities', 'technologies', 'terms'):
            if not isinstance(source[field], list) or not all(isinstance(item, str) for item in source[field]):
                raise ValueError(f'{key}: {field} must be a list of strings')
        if 'summary' in source and (not isinstance(source['summary'], str) or not source['summary'].strip()):
            raise ValueError(f'{key}: summary must be a nonempty string')
        if 'use_when' in source and (not isinstance(source['use_when'], list) or not all(isinstance(item, str) for item in source['use_when'])):
            raise ValueError(f'{key}: use_when must be a list of strings')
        if source['kind'] == 'npm':
            name = source['package']
            if not re.fullmatch(r'(?:@[a-z0-9._-]+/)?[a-z0-9._-]+', name) or any(part in ('.', '..') for part in name.split('/')):
                raise ValueError(f'Unsafe npm package name: {name}')
        if source['kind'] == 'go':
            relative(source['module'])
        if source['kind'] == 'vendored':
            relative(source['directory'])
            pattern = source['pattern']
            if '/' in pattern or '\\' in pattern or '..' in pattern:
                raise ValueError(f'Unsafe vendored glob: {pattern}')
            for name in source.get('companion_files', []):
                relative(name)
    for key, enhancement in overlay.get('enhancements', {}).items():
        identifier(key)
        for field in ('use_when', 'terms'):
            if not isinstance(enhancement[field], list) or not all(isinstance(item, str) for item in enhancement[field]):
                raise ValueError(f'{key}: {field} must be a list of strings')


def managed_base(library, base=None):
    library = library.resolve(strict=True)
    releases = library / 'releases'
    if releases.is_symlink() or not releases.is_dir():
        raise ValueError('Expected a real managed releases directory')
    current = library / 'current'
    if not current.is_symlink():
        raise ValueError('Expected a managed current-release symlink')
    active = current.resolve(strict=True)
    if active.parent != releases:
        raise ValueError('Current release is outside the managed releases directory')
    identifier(active.name)
    selected = active if base is None else Path(base)
    if not selected.is_absolute():
        selected = releases / identifier(str(selected))
    selected = selected.resolve(strict=True)
    if selected.parent != releases:
        raise ValueError('Base release is outside the managed releases directory')
    identifier(selected.name)
    release = read_json(selected / 'release.json')
    for filename, field in HASH_FILES.items():
        if field in release and sha256(selected / filename) != release[field]:
            raise ValueError(f'Base release hash mismatch: {filename}')
    return library, selected, active, release


def npm_repository(value):
    if isinstance(value, dict):
        value = value.get('url')
    if not isinstance(value, str) or not value:
        raise ValueError('Installed npm manifest must declare a repository URL')
    value = value.removeprefix('git+')
    if value.startswith('github:'):
        value = 'https://github.com/' + value[len('github:'):]
    if value.startswith('git://'):
        value = 'https://' + value[len('git://'):]
    if value.startswith('git@github.com:'):
        value = 'https://github.com/' + value[len('git@github.com:'):]
    if not value.startswith(('https://', 'http://')):
        if re.fullmatch(r'[\w.-]+/[\w.-]+', value):
            value = 'https://github.com/' + value
        else:
            raise ValueError(f'Unsupported npm repository URL: {value}')
    value = value.split('#', 1)[0].rstrip('/')
    return value if value.endswith('.git') else value + '.git'


def module_location(project, source, go_cache):
    module = source['module']
    graph = json.loads(command(['go', 'mod', 'edit', '-json'], project))
    requirement = next((entry for entry in (graph.get('Require') or []) if entry['Path'] == module), None)
    if requirement is None:
        raise ValueError(f'Go module is not required by project go.mod: {module}')
    version = requirement['Version']
    replacement = next((entry['New'] for entry in (graph.get('Replace') or [])
                        if entry['Old']['Path'] == module and entry['Old'].get('Version', '') in ('', version)), None)
    actual_module = replacement['Path'] if replacement else module
    actual_version = replacement.get('Version', '') if replacement else version
    if not actual_version:
        raise ValueError(f'Local Go replacement needs a separate source definition: {module}')
    if not re.fullmatch(r'v[0-9A-Za-z.+-]+', actual_version):
        raise ValueError(f'Unsafe Go module version: {actual_version}')
    relative(actual_module)
    cache = Path(go_cache or command(['go', 'env', 'GOMODCACHE'], project)).resolve(strict=True)
    escaped = re.sub(r'[A-Z]', lambda match: '!' + match.group().lower(), actual_module)
    location = within(cache / f'{escaped}@{actual_version}', cache)
    return location, actual_version, {'module': module, 'project_requirement': version,
                                      'cached_module': actual_module}


def archive_head(project, destination):
    revision = command(['git', 'rev-parse', '--verify', 'HEAD^{commit}'], project)
    destination.mkdir()
    process = subprocess.Popen(['git', '-C', str(project), 'archive', '--format=tar', revision], stdout=subprocess.PIPE)
    try:
        with tarfile.open(fileobj=process.stdout, mode='r|') as archive:
            for member in archive:
                path = relative(member.name.rstrip('/'))
                target = destination / path
                if member.isdir():
                    target.mkdir(parents=True, exist_ok=True)
                elif member.isfile():
                    target.parent.mkdir(parents=True, exist_ok=True)
                    with archive.extractfile(member) as stream, target.open('wb') as output:
                        shutil.copyfileobj(stream, output)
                    target.chmod(0o755 if member.mode & 0o111 else 0o644)
                else:
                    # Do not extract links, devices, or submodule placeholders.
                    raise ValueError(f'Unsupported Git archive entry: {member.name}')
        if process.wait() != 0:
            raise ValueError('git archive failed')
    except BaseException:
        process.kill()
        process.wait()
        raise
    finally:
        process.stdout.close()
    return revision


def snapshot(project, definition, destination, go_cache):
    kind = definition['kind']
    url = definition.get('url')
    license_name = 'unspecified'
    if kind == 'npm':
        package = definition['package']
        modules = project / 'node_modules'
        location = within(modules / package, modules)
        manifest = read_json(location / 'package.json')
        if manifest.get('name') != package:
            raise ValueError(f'Installed package name mismatch: {package}')
        version = manifest['version']
        url = npm_repository(manifest.get('repository'))
        license_name = manifest.get('license', 'unspecified')
        if isinstance(license_name, dict):
            license_name = license_name.get('type', 'unspecified')
        requirements = read_json(project / 'package.json')
        specs = {**requirements.get('devDependencies', {}), **requirements.get('dependencies', {})}
        if package not in specs:
            raise ValueError(f'npm source is not a project dependency: {package}')
        origin = {'package': package, 'project_requirement': specs[package]}
        copy_local(location, destination)
    elif kind == 'go':
        location, version, origin = module_location(project, definition, go_cache)
        copy_local(location, destination)
    elif kind == 'vendored':
        directory = within(project / relative(definition['directory']), project)
        matches = sorted(directory.glob(definition['pattern']))
        if len(matches) != 1:
            raise ValueError('Expected exactly one vendored Datastar runtime')
        runtime = within(matches[0], directory)
        match = re.fullmatch(r'datastar-(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\.js', runtime.name)
        if not match:
            raise ValueError('Cannot derive Datastar version from vendored filename')
        version = match[1]
        origin = {'project_relative_path': runtime.relative_to(project).as_posix()}
        destination.mkdir()
        for name in [runtime.name, *definition.get('companion_files', [])]:
            path = relative(name)
            target = destination / path
            target.parent.mkdir(parents=True, exist_ok=True)
            copy_local(directory / path, target, directory)
    else:
        version = archive_head(project, destination)
        origin = {'revision': version}
    measured = content_info(destination)
    if measured['file_count'] == 0:
        raise ValueError(f'Local snapshot is empty: {definition["id"]}')
    return {'kind': KINDS[kind], 'version': version, 'origin': origin, **measured}, url, license_name


def source_entry(definition, provenance, url, license_name):
    kind = definition['kind']
    entry = {'id': definition['id'], 'title': definition['title'],
             'summary': definition.get('summary', f'{definition["title"]}: locally sourced {KINDS[kind]} reference for LeapView.'),
             'type': 'git', 'url': url, 'topics': definition['topics'],
             'capabilities': definition['capabilities'], 'technologies': definition['technologies'],
             'reference_roles': ['dependency', 'implementation-reference'],
             'artifact_types': ['source-code', 'documentation'],
             'use_when': definition.get('use_when', [f'Working on {definition["owner"]}']), 'suggested_terms': definition['terms'],
             'authority': 'upstream', 'adoption': 'adopted', 'priority': 'high',
             # The catalog schema declares the upstream retrieval strategy. Actual
             # installed/cached/archived copy provenance lives in the local lock.
             'acquisition': {'method': 'git'},
             'license': {'status': 'review-required', 'name': license_name,
                         'notes': 'Local dependency snapshot; upstream mirror approval is not inferred.'},
             'generated': {key: provenance[key] for key in ('file_count', 'size_bytes')}}
    if kind == 'project':
        entry.update(authority='internal', reference_roles=['implementation-reference', 'documentation'], priority='critical')
        entry['generated']['revision'] = provenance['version']
        entry['license'] = {'status': 'review-required', 'notes': 'First-party tracked-source snapshot of Git HEAD.'}
    return entry


def markdown(catalog, lock):
    lines = ['# Flid reference catalog', '', f'{len(catalog["sources"])} reference entries.', '',
             'Upstream acquisitions retain their original lock. Local snapshots have a separate provenance lock.', '']
    for source in catalog['sources']:
        lines += [f'## {source["title"]}', '', f'- **ID:** `{source["id"]}`', f'- **Summary:** {source.get("summary", "")}']
        for field in ('topics', 'capabilities', 'reference_roles', 'technologies', 'use_when', 'suggested_terms'):
            lines.append(f'- **{field}:** {", ".join(source.get(field, []))}')
        lines += [f'- **Upstream:** {source.get("url", "")}', f'- **Local path:** `{source["path"]}`']
        if source['id'] in lock['sources']:
            provenance = lock['sources'][source['id']]
            lines += [f'- **Local snapshot:** {provenance["kind"]}, version `{provenance["version"]}`',
                      f'- **Content SHA-256:** `{provenance["local_content_sha256"]}`']
        lines.append('')
    return '\n'.join(lines)


def prepare(project, library, overlay, stage, base=None, go_cache=None):
    project = Path(project).resolve(strict=True)
    validate_overlay(overlay)
    library, base, active, release = managed_base(Path(library), base)
    catalog = copy.deepcopy(read_json(base / 'catalog.json'))
    sources = {}
    for source in catalog['sources']:
        key = identifier(source['id'])
        if key in sources:
            raise ValueError(f'Duplicate base source ID: {key}')
        sources[key] = source
    lock_path = base / 'local-references.lock.json'
    lock = copy.deepcopy(read_json(lock_path)) if lock_path.exists() else {'schema_version': 1, 'kind': 'local-reference-provenance', 'sources': {}}
    for key in lock['sources']:
        identifier(key)
        if key not in sources:
            raise ValueError(f'Local provenance lacks a catalog source: {key}')
    # Preserve every non-catalog release artifact, including the upstream lock,
    # executable discovery tool, and its bundled skills.
    for child in sorted(base.iterdir()):
        if child.name not in ('references', 'catalog.json', 'CATALOG.md', 'release.json', 'local-references.lock.json'):
            copy_local(child, stage / child.name, base)
    references = stage / 'references'
    references.mkdir()
    replacing = {source['id'] for source in overlay['sources'] if source['id'] not in sources or source['id'] in lock['sources']}
    for key in sources:
        if key in replacing:
            continue
        location = base / 'references' / key
        if location.exists():
            actual = location.resolve(strict=True)
            # Inherited references may point to earlier immutable releases, but
            # only to the same source ID inside the managed releases directory.
            if actual.parent.name != 'references' or actual.name != key or actual.parent.parent.parent != library / 'releases':
                raise ValueError(f'Inherited reference escapes managed releases: {key}')
            if key in lock['sources']:
                copy_local(actual, references / key)
                measured = content_info(references / key)
                if any(lock['sources'][key].get(field) != value for field, value in measured.items()):
                    raise ValueError(f'Inherited local snapshot hash mismatch: {key}')
            else:
                (references / key).symlink_to(actual, target_is_directory=True)
        elif key in lock['sources']:
            raise ValueError(f'Missing inherited local snapshot: {key}')
    for section, additions in overlay.get('taxonomy', {}).items():
        target = catalog.setdefault('taxonomy', {}).setdefault(section, {})
        for key, value in additions.items():
            target.setdefault(key, value)
    added = refreshed = preserved = guidance = 0
    for definition in overlay['sources']:
        key = definition['id']
        if key not in replacing:
            preserved += 1
            continue
        provenance, url, license_name = snapshot(project, definition, references / key, go_cache)
        if key in sources:
            refreshed += 1
        else:
            added += 1
        sources[key] = source_entry(definition, provenance, url, license_name)
        lock['sources'][key] = provenance
    for key, enhancement in overlay.get('enhancements', {}).items():
        if key not in sources:
            raise ValueError(f'Guidance source does not exist in the base catalog: {key}')
        changed = False
        for field, additions in [('use_when', enhancement['use_when']), ('suggested_terms', enhancement['terms'])]:
            target = sources[key].setdefault(field, [])
            for item in additions:
                if item not in target:
                    target.append(item)
                    changed = True
        guidance += int(changed)
    catalog['root'] = str(library / 'current')
    catalog['sources'] = [sources[key] for key in sorted(sources)]
    for key, source in sources.items():
        source['path'] = str(library / 'current' / 'references' / key)
    for key, provenance in lock['sources'].items():
        provenance['path'] = sources[key]['path']
    dump(stage / 'catalog.json', catalog)
    # Use the deployed consumer to validate the entire candidate catalog before
    # freezing or switching current. A structurally valid JSON file is not enough.
    checked = subprocess.run([str(stage / 'bin/flid-ref'), 'show', catalog['sources'][0]['id'],
                              '--catalog', str(stage / 'catalog.json'), '--json'],
                             stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    if checked.returncode:
        raise ValueError(f'Discovery tool rejected candidate catalog: {checked.stderr.strip()}')
    dump(stage / 'local-references.lock.json', lock)
    (stage / 'CATALOG.md').write_text(markdown(catalog, lock), encoding='utf-8')
    hashes = {field: sha256(stage / filename) for filename, field in HASH_FILES.items()}
    # Content identity excludes the immediate base ID: applying the same overlay
    # to its own result must identify the same immutable release.
    artifact_hashes = {}
    for child in stage.iterdir():
        if child.name not in ('references', 'catalog.json', 'CATALOG.md', 'local-references.lock.json'):
            artifact_hashes[child.name] = content_info(child)['local_content_sha256'] if child.is_dir() else sha256(child)
    identity = json.dumps({'hashes': hashes, 'artifacts': artifact_hashes}, sort_keys=True).encode()
    release_id = hashlib.sha256(identity).hexdigest()[:20] + '-local'
    updated = dict(release, release_id=release_id, release_kind='local-overlay',
                   base_release_id=release.get('base_release_id', release['release_id']),
                   catalog_entry_count=len(sources), local_reference_count=len(lock['sources']),
                   artifact_sha256=artifact_hashes, **hashes)
    # Totals are derived from the catalog, avoiding double-counting on reapply.
    updated['reference_file_count'] = sum(source.get('generated', {}).get('file_count', 0) for source in sources.values())
    updated['content_bytes'] = sum(source.get('generated', {}).get('size_bytes', 0) for source in sources.values())
    updated.pop('local_created_at', None)
    dump(stage / 'release.json', updated)
    return {'release_id': release_id, 'base_release_id': release['release_id'],
            'catalog_before': len(read_json(base / 'catalog.json')['sources']), 'catalog_after': len(sources),
            'sources_added': added, 'local_sources_refreshed': refreshed, 'existing_sources_preserved': preserved,
            'guidance_updates': guidance, 'local_reference_count': len(lock['sources']),
            'upstream_snapshots_unchanged': True, 'upstream_lock_sha256': hashes['lock_sha256']}, active


def remove_stage(stage):
    if stage.exists():
        for directory, _, _ in os.walk(stage, followlinks=False):
            Path(directory).chmod(0o755)
        shutil.rmtree(stage)


def relocate_review_catalog(stage, output):
    """Bind review metadata to its output while retaining the planned release ID."""
    catalog = read_json(stage / 'catalog.json')
    lock = read_json(stage / 'local-references.lock.json')
    catalog['root'] = str(output)
    for source in catalog['sources']:
        source['path'] = str(output / 'references' / source['id'])
    for key, provenance in lock['sources'].items():
        provenance['path'] = str(output / 'references' / key)
    dump(stage / 'catalog.json', catalog)
    dump(stage / 'local-references.lock.json', lock)
    (stage / 'CATALOG.md').write_text(markdown(catalog, lock), encoding='utf-8')
    release = read_json(stage / 'release.json')
    release.update(release_kind='review-candidate', **{field: sha256(stage / filename) for filename, field in HASH_FILES.items()})
    dump(stage / 'release.json', release)


def build(project, library, overlay, output, base=None, go_cache=None):
    """Build a reviewable candidate at a new directory without changing current."""
    output = Path(output).absolute()
    if output.exists() or output.is_symlink():
        raise ValueError(f'Build output already exists: {output}')
    output.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix='.reference-build-', dir=output.parent))
    try:
        report, _ = prepare(project, library, overlay, stage, base, go_cache)
        relocate_review_catalog(stage, output)
        stage.rename(output)
        return report
    finally:
        remove_stage(stage)


def freeze(stage):
    # The top-level directory remains writable until after its rename. Making
    # it read-only first prevents rename on some managed filesystems.
    for path in stage.rglob('*'):
        if not path.is_symlink():
            path.chmod(0o555 if path.is_dir() or path.stat().st_mode & 0o111 else 0o444)


def install(project, library, overlay, base=None, go_cache=None):
    """Serialize preparation and activation so each install sees the latest base."""
    library = Path(library).resolve(strict=True)
    descriptor = os.open(library / '.install.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        return _install_locked(project, library, overlay, base, go_cache)


def _install_locked(project, library, overlay, base=None, go_cache=None):
    """Validate, install an immutable local release, and atomically switch current."""
    library, _, initial_active, _ = managed_base(Path(library), base)
    incoming = library / '.incoming'
    if incoming.is_symlink():
        raise ValueError('Staging directory must not be a symlink')
    incoming.mkdir(exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix='reference-overlay-', dir=incoming))
    link = None
    try:
        report, active = prepare(project, library, overlay, stage, base, go_cache)
        if active != initial_active or (library / 'current').resolve(strict=True) != active:
            raise ValueError('Current release changed during preparation; retry against the new base')
        release_id = report['release_id']
        final = library / 'releases' / release_id
        if final.exists() or final.is_symlink():
            if final.is_symlink():
                raise ValueError('Existing release must not be a symlink')
            if final.stat().st_mode & 0o222:
                raise ValueError('Existing release directory is writable')
            expected = read_json(stage / 'release.json')
            recorded = read_json(final / 'release.json')
            for filename, field in HASH_FILES.items():
                if (final / filename).is_symlink() or recorded.get(field) != expected[field] or sha256(final / filename) != expected[field]:
                    raise ValueError(f'Existing release differs from candidate: {filename}')
            if recorded.get('release_id') != release_id or recorded.get('artifact_sha256') != expected['artifact_sha256']:
                raise ValueError('Existing release artifact manifest differs from candidate')
            for name, expected_hash in expected['artifact_sha256'].items():
                artifact = final / relative(name)
                if artifact.is_symlink():
                    raise ValueError(f'Existing release artifact is a symlink: {name}')
                actual_hash = content_info(artifact)['local_content_sha256'] if artifact.is_dir() else sha256(artifact)
                if actual_hash != expected_hash:
                    raise ValueError(f'Existing release artifact differs: {name}')
            for key, provenance in read_json(final / 'local-references.lock.json')['sources'].items():
                actual = content_info(final / 'references' / identifier(key))
                if any(provenance.get(field) != value for field, value in actual.items()):
                    raise ValueError(f'Existing release snapshot differs: {key}')
        else:
            freeze(stage)
            stage.rename(final)
            final.chmod(0o555)
        # Creating a uniquely named sibling avoids trampling other install jobs.
        fd, temporary = tempfile.mkstemp(prefix='.current-', dir=library)
        os.close(fd)
        link = Path(temporary)
        link.unlink()
        link.symlink_to(Path('releases') / release_id, target_is_directory=True)
        if (library / 'current').resolve(strict=True) != active:
            raise ValueError('Current release changed before activation; retry')
        os.replace(link, library / 'current')
        report['status'] = 'installed'
        return report
    finally:
        if link is not None:
            link.unlink(missing_ok=True)
        remove_stage(stage)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['plan', 'build', 'install'])
    parser.add_argument('--project-root', type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--library-root', type=Path, default=Path('/srv/flid/reference-library'))
    parser.add_argument('--overlay', type=Path, default=Path(__file__).with_name('reference-library') / 'overlay.json')
    parser.add_argument('--base', help='Managed release name or absolute path (defaults to current)')
    parser.add_argument('--go-cache', type=Path, help='Override the Go module cache (default: go env GOMODCACHE)')
    parser.add_argument('--output', type=Path, help='New candidate directory (required for build)')
    args = parser.parse_args()
    if args.command == 'build' and args.output is None:
        parser.error('build requires --output')
    if args.command != 'build' and args.output is not None:
        parser.error('--output only applies to build')
    try:
        overlay = read_json(args.overlay)
        params = (args.project_root, args.library_root, overlay)
        kwargs = {'base': args.base, 'go_cache': args.go_cache}
        if args.command == 'install':
            report = install(*params, **kwargs)
        elif args.command == 'build':
            report = build(*params, args.output, **kwargs)
        else:
            with tempfile.TemporaryDirectory(prefix='reference-plan-') as temporary:
                report = build(*params, Path(temporary) / 'candidate', **kwargs)
            report['status'] = 'planned'
        print(json.dumps(report, indent=2))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f'reference-library: {error}\n')


if __name__ == '__main__':
    main()
