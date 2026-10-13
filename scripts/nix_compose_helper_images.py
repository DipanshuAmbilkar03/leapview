#!/usr/bin/env python3
"""Prepare native, pinned authoring helpers before the timed fresh-guest journey."""

import argparse
import gzip
import hashlib
import json
import os
import platform as host_platform
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import stat
import subprocess
import tarfile


ASSETS = ("Dockerfile.authoring-client", "Dockerfile.authoring-browser", "package.json",
          "package-lock.json", "authoring-worker.mjs")
ROLES = ("client", "browser")
ARCHIVE = "helper-images.tar.gz"
RECEIPT = "helper-receipt.json"
MAX_ARCHIVE = 6 * 1024**3
MAX_EXPANDED = 12 * 1024**3
SHA256 = re.compile(r"sha256:[0-9a-f]{64}\Z")
REVISION = re.compile(r"[0-9a-f]{40}\Z")


class HelperImageError(ValueError):
    pass


def _regular(path: Path, limit: int) -> None:
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= limit:
        raise HelperImageError("helper input must be a bounded regular file")


def file_digest(path: Path) -> str:
    with path.open("rb") as source:
        return "sha256:" + hashlib.file_digest(source, "sha256").hexdigest()


def asset_digests(root: Path) -> dict[str, str]:
    directory = root / "deploy/compose/qualification"
    if not stat.S_ISDIR(directory.lstat().st_mode) or not directory.resolve().is_relative_to(root.resolve()):
        raise HelperImageError("helper assets must be inside the protected checkout")
    result = {}
    for name in ASSETS:
        path = directory / name
        _regular(path, 8 * 1024**2)
        result[name] = file_digest(path)
    return result


def image_tags(revision: str, platform: str) -> dict[str, str]:
    if not REVISION.fullmatch(revision) or platform not in {"linux/amd64", "linux/arm64"}:
        raise HelperImageError("invalid helper protected revision or platform")
    return {role: f"leapview-qualification-{role}:{revision}-{platform.split('/')[1]}" for role in ROLES}


def archive_images(path: Path, tags: dict[str, str], platform: str) -> dict[str, str]:
    """Read canonical config digests from Docker save, independent of host image-store IDs."""
    _regular(path, MAX_ARCHIVE)
    if set(tags) != set(ROLES) or len(set(tags.values())) != 2 or platform not in {"linux/amd64", "linux/arm64"}:
        raise HelperImageError("invalid helper roles or platform")
    with tarfile.open(path, "r:gz") as archive:
        members, total = {}, 0
        for member in archive:
            name = PurePosixPath(member.name)
            total += member.size
            if (name.is_absolute() or ".." in name.parts or "\\" in member.name
                    or member.name in members or not (member.isfile() or member.isdir())
                    or member.size < 0 or total > MAX_EXPANDED or len(members) >= 4096):
                raise HelperImageError("unsafe or oversized helper archive")
            members[member.name] = member

        def read_json(name):
            member = members.get(name)
            if member is None or not member.isfile() or not 0 < member.size <= 2 * 1024**2:
                raise HelperImageError("helper archive metadata is absent or oversized")
            with archive.extractfile(member) as source:
                raw = source.read()
            return raw, json.loads(raw)

        _, manifest = read_json("manifest.json")
        if not isinstance(manifest, list) or len(manifest) != 2:
            raise HelperImageError("helper archive must contain exactly two images")
        result = {}
        for role, tag in tags.items():
            matches = [item for item in manifest if isinstance(item, dict) and item.get("RepoTags") == [tag]]
            if len(matches) != 1:
                raise HelperImageError("helper archive does not bind the expected roles")
            item = matches[0]
            if not isinstance(item.get("Config"), str):
                raise HelperImageError("helper archive configuration is absent")
            raw, config = read_json(item["Config"])
            if not isinstance(config, dict) or config.get("os") != "linux" or config.get("architecture") != platform.split('/')[1]:
                raise HelperImageError("helper image platform differs from the native guest")
            rootfs = config.get("rootfs", {})
            layers = item.get("Layers")
            diff_ids = rootfs.get("diff_ids") if isinstance(rootfs, dict) else None
            if (not isinstance(layers, list) or not layers or not isinstance(diff_ids, list)
                    or len(layers) != len(diff_ids) or rootfs.get("type") != "layers"
                    or any(not isinstance(value, str) or not SHA256.fullmatch(value) for value in diff_ids)
                    or any(not isinstance(name, str) or name not in members or not members[name].isfile() for name in layers)):
                raise HelperImageError("helper archive layer inventory is invalid")
            result[role] = "sha256:" + hashlib.sha256(raw).hexdigest()
        if len(set(result.values())) != 2:
            raise HelperImageError("client and browser helpers must have distinct configurations")
        return result


def validate_receipt(receipt: dict, *, source_revision: str, protected_revision: str,
                     image: str, platform: str, assets: dict[str, str] | None = None) -> dict:
    expected = {"schemaVersion", "scope", "result", "sourceRevision", "protectedRevision", "image",
                "platform", "archiveSHA256", "assetsSHA256", "images"}
    if not isinstance(receipt, dict) or set(receipt) != expected:
        raise HelperImageError("unsupported helper receipt shape")
    if (type(receipt["schemaVersion"]) is not int or receipt["schemaVersion"] != 1 or receipt["scope"] != "native-compose-authoring-helpers"
            or receipt["result"] != "passed" or receipt["sourceRevision"] != source_revision
            or receipt["protectedRevision"] != protected_revision or receipt["image"] != image
            or receipt["platform"] != platform or not REVISION.fullmatch(source_revision)
            or not REVISION.fullmatch(protected_revision)):
        raise HelperImageError("helper receipt identity differs from the selected qualification")
    tags = image_tags(protected_revision, platform)
    if (not isinstance(receipt["archiveSHA256"], str) or not SHA256.fullmatch(receipt["archiveSHA256"])
            or not isinstance(receipt["assetsSHA256"], dict) or set(receipt["assetsSHA256"]) != set(ASSETS)
            or any(not isinstance(value, str) or not SHA256.fullmatch(value) for value in receipt["assetsSHA256"].values())
            or (assets is not None and receipt["assetsSHA256"] != assets)
            or not isinstance(receipt["images"], dict) or set(receipt["images"]) != set(tags)
            or any(not isinstance(value, str) or not SHA256.fullmatch(value) for value in receipt["images"].values())
            or len(set(receipt["images"].values())) != 2):
        raise HelperImageError("helper receipt artifact or asset bindings are invalid")
    return receipt


def verify_preparation(directory: Path, **identity) -> dict:
    receipt_path, archive = directory / RECEIPT, directory / ARCHIVE
    _regular(receipt_path, 64 * 1024)
    receipt = validate_receipt(json.loads(receipt_path.read_bytes()), **identity)
    _regular(archive, MAX_ARCHIVE)
    if file_digest(archive) != receipt["archiveSHA256"]:
        raise HelperImageError("helper archive checksum differs from its preparation receipt")
    images = archive_images(archive, image_tags(identity["protected_revision"], identity["platform"]), identity["platform"])
    if images != receipt["images"]:
        raise HelperImageError("helper archive configuration identities differ from the receipt")
    return receipt


def prepare(args) -> dict:
    # Import lazily: the guest collector also uses this module to verify prepared inputs.
    import nix_compose_host_guest as host_guest
    host_guest._protected_checkout_revision(args.protected_root, args.protected_revision)
    architecture = {"x86_64": "amd64", "aarch64": "arm64"}.get(host_platform.machine())
    if host_platform.system() != "Linux" or args.platform != "linux/" + str(architecture):
        raise HelperImageError("helper preparation requires the matching native Linux architecture")
    if not REVISION.fullmatch(args.source_revision) or re.fullmatch(r"ghcr\.io/flidai/leapview@sha256:[0-9a-f]{64}", args.image) is None:
        raise HelperImageError("helper preparation requires the selected immutable application identity")
    assets = asset_digests(args.protected_root)
    tags = image_tags(args.protected_revision, args.platform)
    args.output_dir.mkdir(parents=True, exist_ok=False)
    context = args.output_dir / "context"
    context.mkdir()
    for name in ASSETS:
        shutil.copyfile(args.protected_root / "deploy/compose/qualification" / name, context / name)
        if file_digest(context / name) != assets[name]:
            raise HelperImageError("protected helper asset changed while copied")
    try:
        for role, tag in tags.items():
            command = ["docker", "build", "--platform", args.platform, "--file", str(context / ("Dockerfile.authoring-" + role)), "--tag", tag]
            if role == "client":
                command += ["--build-arg", "LEAPVIEW_IMAGE=" + args.image]
            subprocess.run(command + [str(context)], stdin=subprocess.DEVNULL, check=True, timeout=1800)
        archive = args.output_dir / ARCHIVE
        process = subprocess.Popen(["timeout", "--kill-after=10", "1800", "docker", "image", "save", *tags.values()],
                                   stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, start_new_session=True)
        try:
            with archive.open("xb") as output, gzip.GzipFile(fileobj=output, mode="wb", compresslevel=1, mtime=0) as compressed:
                exported = 0
                while chunk := process.stdout.read(1024 * 1024):
                    exported += len(chunk)
                    if exported > MAX_EXPANDED or output.tell() > MAX_ARCHIVE:
                        raise HelperImageError("native helper image export exceeds its byte limit")
                    compressed.write(chunk)
            if process.wait(timeout=30) != 0:
                raise HelperImageError("native helper image export failed")
        finally:
            process.stdout.close()
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
        images = archive_images(archive, tags, args.platform)
        receipt = {"schemaVersion": 1, "scope": "native-compose-authoring-helpers", "result": "passed",
                   "sourceRevision": args.source_revision, "protectedRevision": args.protected_revision,
                   "image": args.image, "platform": args.platform, "archiveSHA256": file_digest(archive),
                   "assetsSHA256": assets, "images": images}
        (args.output_dir / RECEIPT).write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n")
        return receipt
    finally:
        shutil.rmtree(context)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--protected-root", type=Path, required=True)
    parser.add_argument("--protected-revision", required=True)
    parser.add_argument("--source-revision", required=True)
    parser.add_argument("--image", required=True)
    parser.add_argument("--platform", choices=("linux/amd64", "linux/arm64"), required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    try:
        print(json.dumps(prepare(parser.parse_args()), sort_keys=True))
    except (HelperImageError, OSError, subprocess.SubprocessError, tarfile.TarError, ValueError, KeyError, TypeError) as exc:
        raise SystemExit("Compose helper preparation rejected: " + str(exc)) from exc


if __name__ == "__main__":
    main()
