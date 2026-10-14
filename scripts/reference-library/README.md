# Flid reference library maintenance

This overlay adds implementation references for LeapView's current browser,
agent, and review tooling to the locally installed Flid reference library.
The original September 8 catalog has 52 entries; applying the overlay adds 21
local references and expands lookup guidance for six existing references.
Reapplying it refreshes the local files without duplicating entries or guidance.

The [LeapView source guide](leapview/README.md) maps current behavior to implementation
paths and merged PRs. The [complete PR inventory](leapview/merged-prs.md) records
449 merges since September 8, with dates, target branches, commit IDs and ancestry
in [machine-readable form](leapview/merged-prs.json). The guide and inventory are
included in the actual installed `references/leapview` snapshot, not only the
Playground. The catalog points agents directly to the guide.

`overlay.json` is the shared source for the maintenance script and the Playground
reference page. The script resolves exact npm versions and repository URLs from
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
inventing an upstream commit or fetch date. Catalog `acquisition.method: git` is
the upstream retrieval strategy accepted by the installed discovery-tool schema;
the actual installed-package/module/archive copy method is recorded in
`local-references.lock.json`. The bundled `flid-ref` checks every candidate catalog
before installation can switch `current`. Existing license decisions remain
unchanged; new entries record declared package licenses and retain
`review-required` status for upstream mirroring.

## Review the changes

Run `task playground` and open `/#recipes/reference-refresh` to browse the
project references, search their lookup terms and read reference guidance.
The page reads committed metadata and project requirements.

The [reference-page screenshot](screenshots/flid-reference.png) shows the current
page layout.

Run the maintenance regression checks with:

```sh
python3 -B -m unittest discover -s scripts -p reference_library_test.py
```

They cover local provenance, committed source isolation, repeat application,
upstream preservation, immutable atomic installation, failure recovery and unsafe
path rejection. The frontend core CI lane runs them, while the existing
Playground browser suite checks filtering and shared search state.

## Refresh after project changes

Use a clean, reviewed checkout containing the merged changes you want to reference.
The installer archives committed `HEAD`, so commit guide/inventory edits before
installation. It never silently substitutes an old checkout's files with remote
main or includes uncommitted work.

1. Fetch the repository and incorporate the reviewed main revision in your checkout.
2. Review changed source and update `leapview/README.md`; keep reverted proposals
   and unmerged work distinct from current behavior.
3. Refresh the PR inventory from GitHub and record the review date, exact main
   revision, target branch and merge ancestry. The current snapshot query is:

   ```sh
   gh pr list --repo flidai/leapview --state merged \
     --search 'merged:>=2026-09-08' --limit 1000 \
     --json number,title,mergedAt,url,mergeCommit,baseRefName
   ```

   If the result reaches the requested limit, paginate before claiming completeness.
   A merge into another branch or a subsequently reverted PR is historical evidence,
   not proof of current behavior.
4. Commit the updated references, run the maintenance regression checks, then run
   `python3 -B scripts/reference_library.py plan` followed by `install`.
5. Check the installed catalog's LeapView revision and open its guide through
   `/srv/flid/reference-library/current/references/leapview/scripts/reference-library/leapview/README.md`.
   Use the same `flid-ref discover` command as the existing skill:

   ```sh
   /srv/flid/reference-library/current/bin/flid-ref discover "credential activation" \
     --catalog /srv/flid/reference-library/current/catalog.json --json
   ```

This workflow updates this host. Installing the PR on another host requires running
the maintenance command there with its available local dependencies and Go cache.
