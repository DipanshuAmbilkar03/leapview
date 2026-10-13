# Flid reference library maintenance

This overlay adds implementation references for LeapView's current browser,
agent, and review tooling to the locally installed Flid reference library.
The original September 8 catalog has 52 entries; applying the overlay adds 21
local references and expands lookup guidance for six existing references.
Reapplying it refreshes the local files without duplicating entries or guidance.

`overlay.json` is the shared source for the maintenance script and the Playground
comparison. The script resolves exact npm versions and repository URLs from
installed packages, Go versions from `go.mod` and the module cache, the Datastar
version from its vendored filename, and first-party source from committed Git
HEAD. Uncommitted files and local edits are excluded from that Git archive.

## Inspect and install

Use Python 3.11 or newer, Git, Go, the project's installed Bun dependencies, and
an existing managed Flid library with its `current` symlink and `releases`
directory. Go dependencies must already exist in the local module cache. These
commands use local inputs and disable Go proxy and toolchain downloads.

```sh
# Inspect a candidate without changing the installed library.
python3 -B scripts/reference_library.py plan

# Keep a candidate and its copied files for review at a new directory.
python3 -B scripts/reference_library.py build --output .tmp/reference-library/candidate

# Install a validated local release; requires write access to the library root.
python3 -B scripts/reference_library.py install
```

The library defaults to `/srv/flid/reference-library`; the project defaults to
this checkout. Override them with `--library-root` and `--project-root`. Use
`--base RELEASE_NAME` to build against a particular retained managed release,
and `--go-cache` to select a local module cache. A build output must not already
exist.

Installation creates independent, measured copies of the added references,
validates release files, makes the release read-only, and switches `current`
atomically. The previous release remains under `releases/` for rollback. A
preparation or activation failure leaves the current release unchanged. Existing
upstream reference snapshots and `knowledge.lock.json` are preserved; local file
origins and checksums live in a separate `local-references.lock.json`.

Local distributions can contain different files from full upstream repositories.
Their versions, copy methods, file counts and checksums are recorded without
inventing an upstream commit or fetch date. Existing license decisions remain
unchanged; new entries record declared package licenses and retain
`review-required` status for upstream mirroring.

## Review the changes

Run `task playground` and open `/#recipes/reference-refresh` to compare the
historical catalog coverage, browse the additions, search their lookup terms,
and review expanded guidance. This example reads the committed overlay and
project requirements; it does not inspect or change the installed library.

The committed screenshots show the [before](screenshots/reference-before.png) and
[after](screenshots/reference-update.png) comparison. They illustrate catalog
coverage, rather than live installation status.

Run the maintenance regression checks with:

```sh
python3 -B -m unittest discover -s scripts -p reference_library_test.py
```

They cover local provenance, committed source isolation, repeat application,
upstream preservation, immutable atomic installation, failure recovery and unsafe
path rejection. The frontend core CI lane runs them, while the existing
Playground browser suite checks comparison, filtering and shared control state.
