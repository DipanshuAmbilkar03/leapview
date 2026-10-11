# Public manifest repair — 11 October 2026

The supported site operator deployed the admitted image from protected main
`ad0c2aba2dce2bbb1cb8a219623d8f9ad505e801`:
`ghcr.io/flidai/leapview-site@sha256:cb61e47659c597a4cf1eb64d76fd1e02b101994c88a654e4857881982debf51e`.
[Image build](https://github.com/flidai/leapview/actions/runs/38120996675) and
[hosted public-install smoke](https://github.com/flidai/leapview/actions/runs/38122663189)
both passed. Live `/desktop-release.json` and `/release.json` matched the source
manifests. Desktop remains withdrawn and the advertised server remains
`v0.3.0-alpha.1`; this operation did not publish the next release.

The operator repeated admission, registered exact-image cold capacity, deployed,
verified acceptance, and completed maintenance. Its final status retained a
verified prior image with no pending deployment or maintenance.

Cold qualification exposed two local harness defects: containerd snapshot
submounts prevented cleanup, and a mount-only namespace allowed Docker's
`--bridge=none` to remove the host's default bridge. The local bridge and its
existing endpoints were restored without restarting the primary Docker daemon.
The corrected harness requires a private network namespace and recursively
unmounts the private filesystem without lazy or forced detachment.

A fresh qualification with private mount/network namespaces and slirp4netns
egress passed cold pull, active/candidate boot, rollback, candidate restart and
retained-image cleanup. Peak use was 1,677,348,864 bytes and 1,821 inodes; the
largest compressed image was 265,013,233 bytes. The host bridge remained intact.
A real mount-only invocation was rejected before creating an output directory.
All 115 focused operator tests passed. This repeats capacity qualification,
not the earlier production adoption interval or host-reboot drill.

Private receipts are retained by content hash; they contain operator data and
are not committed:

| Receipt | SHA-256 |
| --- | --- |
| Prepared candidate | `080d1c3da2657efc4d2948218b42b372741f470a1d684a383b41b0fd7710cd24` |
| Original deployment baseline | `deee800a6c070b23092a9e7703011cdd9da1d39457526b29d25dc9fe53a910cb` |
| Repeated capacity measurement in isolated network | `51075e058bfaf3918846114a6e9a52f6c0368ec6fd9bfd286398f9731d56f839` |
| Original capacity registration | `7357dc637dc62730b16ba1ef547b88ccc0e858184c8612443662761331c6a077` |
| Final operator status | `67b1ea1a0c18ad5a3168c82d05a2a85188e8a008ad2dcbb8de86e86176151bd3` |

The repeated isolated measurement used the original three-image baseline to
verify the harness fix. It was not registered against the subsequently changed
production state.
