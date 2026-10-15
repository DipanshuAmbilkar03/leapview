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

import nix_compose_helper_image_probe as image_probe


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


def archive_local_image_ids(path: Path, *, archive_sha256: str, tags: dict[str, str],
                            config_digests: dict[str, str], platform: str) -> dict[str, list[str]]:
    """Authenticate one complete archive stream and prove its immutable image IDs.

    Docker's legacy store identifies an image by its config digest; its
    containerd store can identify the same image by an OCI manifest or index.
    Only tag-bound, single-platform descriptor chains to the prepared config
    are admitted. Unsupported descriptor types provide no additional IDs and
    leave the guest's full Docker-save verification available as a fallback.
    """
    if (not isinstance(archive_sha256, str) or SHA256.fullmatch(archive_sha256) is None
            or set(tags) != set(ROLES) or set(config_digests) != set(ROLES)
            or platform not in {"linux/amd64", "linux/arm64"}
            or any(not isinstance(tags[role], str) or not tags[role] for role in ROLES)
            or len(set(tags.values())) != len(ROLES)
            or any(not isinstance(config_digests[role], str) or SHA256.fullmatch(config_digests[role]) is None for role in ROLES)
            or len(set(config_digests.values())) != len(ROLES)):
        raise HelperImageError("helper archive proof bindings are invalid")

    class AuthenticatedReader:
        def __init__(self, source):
            self.source, self.digest, self.total = source, hashlib.sha256(), 0

        def read(self, size=-1):
            size = 64 * 1024 if size is None or size < 0 else size
            data = self.source.read(min(size, MAX_ARCHIVE + 1 - self.total))
            self.total += len(data)
            if self.total > MAX_ARCHIVE:
                raise HelperImageError("helper archive exceeds its compressed byte limit")
            self.digest.update(data)
            return data

    members, metadata, metadata_bytes, payload_bytes = {}, {}, 0, 0
    try:
        descriptor = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_CLOEXEC", 0))
        with os.fdopen(descriptor, "rb") as source:
            info = os.fstat(source.fileno())
            if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= MAX_ARCHIVE:
                raise HelperImageError("helper archive proof requires a bounded regular file")
            authenticated = AuthenticatedReader(source)
            with gzip.GzipFile(fileobj=authenticated, mode="rb") as expanded:
                bounded = image_probe._BoundedReader(expanded, MAX_EXPANDED)
                with tarfile.open(fileobj=bounded, mode="r|", bufsize=1024 * 1024) as archive:
                    for member in archive:
                        raw_name = member.name[:-1] if member.isdir() and member.name.endswith("/") else member.name
                        name = PurePosixPath(raw_name)
                        payload_bytes += member.size
                        if (len(members) >= image_probe.MAX_MEMBERS or member.size < 0 or payload_bytes > MAX_EXPANDED
                                or not raw_name or name.is_absolute() or name.as_posix() != raw_name
                                or any(part in ("", ".", "..") for part in name.parts) or "\\" in member.name
                                or raw_name in members or not (member.isfile() or member.isdir())
                                or (member.isdir() and member.size != 0)):
                            raise HelperImageError("helper archive proof inventory is unsafe or oversized")
                        members[raw_name] = (member.size, member.isfile())
                        config_name = name.name.removesuffix(".json")
                        required_metadata = (raw_name in {"manifest.json", "index.json", "oci-layout"}
                                             or any(config_name == value[7:] for value in config_digests.values()))
                        small_blob = raw_name.startswith("blobs/sha256/") and member.size <= image_probe.MAX_MEMBER_BYTES
                        if member.isfile() and (required_metadata or small_blob):
                            raw = image_probe._read_member(archive, member, image_probe.MAX_MEMBER_BYTES)
                            if raw_name.startswith("blobs/sha256/") and raw_name != "blobs/sha256/" + hashlib.sha256(raw).hexdigest():
                                raise HelperImageError("helper archive OCI blob digest is invalid")
                            keep = required_metadata
                            if not keep:
                                try:
                                    candidate = json.loads(raw, object_pairs_hook=image_probe._no_duplicate_json_keys)
                                except (UnicodeDecodeError, ValueError, RecursionError):
                                    candidate = None
                                keep = (isinstance(candidate, dict) and candidate.get("schemaVersion") == 2
                                        and candidate.get("mediaType") in {
                                            "application/vnd.oci.image.index.v1+json",
                                            "application/vnd.oci.image.manifest.v1+json",
                                            "application/vnd.docker.distribution.manifest.list.v2+json",
                                            "application/vnd.docker.distribution.manifest.v2+json"})
                            if keep:
                                metadata_bytes += len(raw)
                                if metadata_bytes > image_probe.MAX_METADATA_BYTES:
                                    raise HelperImageError("helper archive proof metadata is oversized")
                                metadata[raw_name] = raw
                        elif member.isfile() and raw_name.startswith("blobs/sha256/"):
                            blob = archive.extractfile(member)
                            digest = hashlib.sha256()
                            size = 0
                            while chunk := blob.read(1024 * 1024):
                                size += len(chunk)
                                digest.update(chunk)
                            if size != member.size or raw_name != "blobs/sha256/" + digest.hexdigest():
                                raise HelperImageError("helper archive OCI blob digest is invalid")
                    # Consume and bound all expanded padding/tail and all compressed
                    # input before authenticating or returning any derived ID.
                    while bounded.read(1024 * 1024):
                        pass
            while authenticated.read(1024 * 1024):
                pass
            if "sha256:" + authenticated.digest.hexdigest() != archive_sha256:
                raise HelperImageError("helper archive checksum differs from its preparation receipt")
    except (OSError, EOFError, tarfile.TarError, image_probe.HelperImageProbeError) as exc:
        raise HelperImageError("helper archive proof is not a bounded readable archive") from exc

    def document(raw, description):
        try:
            return json.loads(raw, object_pairs_hook=image_probe._no_duplicate_json_keys)
        except (TypeError, UnicodeDecodeError, ValueError, RecursionError) as exc:
            raise HelperImageError("helper archive " + description + " is invalid") from exc

    try:
        manifest_configs = image_probe._validate_manifest(metadata.get("manifest.json"), tags, config_digests)
    except (TypeError, image_probe.HelperImageProbeError) as exc:
        raise HelperImageError("helper archive tag/config bindings are invalid") from exc
    saved = document(metadata["manifest.json"], "manifest")
    layers_by_role = {}
    for config_path, role in manifest_configs.items():
        raw = metadata.get(config_path)
        if raw is None or "sha256:" + hashlib.sha256(raw).hexdigest() != config_digests[role]:
            raise HelperImageError("helper archive configuration digest differs from preparation")
        config = document(raw, "configuration")
        if (not isinstance(config, dict) or not isinstance(config.get("os"), str)
                or not isinstance(config.get("architecture"), str)
                or config["os"] + "/" + config["architecture"] != platform):
            raise HelperImageError("helper archive configuration platform differs from preparation")
        entry = next(item for item in saved if item["Config"] == config_path)
        layers = entry.get("Layers")
        rootfs = config.get("rootfs")
        diff_ids = rootfs.get("diff_ids") if isinstance(rootfs, dict) else None
        if (not isinstance(layers, list) or not layers or not isinstance(diff_ids, list)
                or len(layers) != len(diff_ids) or rootfs.get("type") != "layers"
                or any(not isinstance(value, str) or SHA256.fullmatch(value) is None for value in diff_ids)
                or any(not isinstance(name, str) or name not in members or not members[name][1] for name in layers)):
            raise HelperImageError("helper archive layer inventory is invalid")
        layers_by_role[role] = layers

    allowed = {role: {config_digests[role]} for role in ROLES}
    if "oci-layout" not in metadata:
        return {role: sorted(ids) for role, ids in allowed.items()}
    layout = document(metadata["oci-layout"], "OCI layout")
    if layout != {"imageLayoutVersion": "1.0.0"} or "index.json" not in metadata:
        raise HelperImageError("helper archive OCI layout is invalid")
    index_types = {"application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"}
    manifest_types = {"application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json"}
    config_types = {"application/vnd.oci.image.config.v1+json", "application/vnd.docker.container.image.v1+json"}
    expected_os, expected_arch = platform.split("/")

    def descriptor_bytes(value):
        if (not isinstance(value, dict) or not isinstance(value.get("digest"), str)
                or SHA256.fullmatch(value["digest"]) is None or type(value.get("size")) is not int
                or not 0 < value["size"] <= image_probe.MAX_MEMBER_BYTES or "urls" in value):
            raise HelperImageError("helper archive OCI descriptor is invalid")
        raw = metadata.get("blobs/sha256/" + value["digest"][7:])
        if raw is None or len(raw) != value["size"] or "sha256:" + hashlib.sha256(raw).hexdigest() != value["digest"]:
            raise HelperImageError("helper archive OCI descriptor content or size is invalid")
        return raw

    def walk(value, role, depth=0):
        raw = descriptor_bytes(value)
        if depth > 5:
            raise HelperImageError("helper archive OCI descriptor graph exceeds its depth limit")
        media_type = value.get("mediaType")
        if media_type not in index_types | manifest_types:
            return None
        observed_platform = value.get("platform")
        if (observed_platform is not None and (not isinstance(observed_platform, dict)
                or observed_platform.get("os") != expected_os or observed_platform.get("architecture") != expected_arch)):
            raise HelperImageError("helper archive OCI descriptor platform differs from preparation")
        node = document(raw, "OCI descriptor document")
        if (not isinstance(node, dict) or type(node.get("schemaVersion")) is not int or node["schemaVersion"] != 2
                or node.get("mediaType", media_type) != media_type):
            raise HelperImageError("helper archive OCI descriptor document is invalid")
        if media_type in index_types:
            children = node.get("manifests")
            if not isinstance(children, list) or len(children) != 1:
                raise HelperImageError("helper archive OCI index is ambiguous")
            child_ids = walk(children[0], role, depth + 1)
            return None if child_ids is None else child_ids | {value["digest"]}
        config = node.get("config")
        if (not isinstance(config, dict) or config.get("mediaType") not in config_types
                or config.get("digest") != config_digests[role]):
            raise HelperImageError("helper archive OCI manifest does not bind the prepared configuration")
        descriptor_bytes(config)
        layers = node.get("layers")
        if not isinstance(layers, list) or len(layers) != len(layers_by_role[role]):
            raise HelperImageError("helper archive OCI manifest layer inventory is invalid")
        for layer, path in zip(layers, layers_by_role[role]):
            if (not isinstance(layer, dict) or not isinstance(layer.get("digest"), str)
                    or SHA256.fullmatch(layer["digest"]) is None or "blobs/sha256/" + layer["digest"][7:] != path
                    or type(layer.get("size")) is not int or layer["size"] != members[path][0] or "urls" in layer):
                raise HelperImageError("helper archive OCI manifest layer descriptor is invalid")
        return {value["digest"]}

    root = document(metadata["index.json"], "OCI index")
    if (not isinstance(root, dict) or type(root.get("schemaVersion")) is not int or root["schemaVersion"] != 2
            or root.get("mediaType") not in index_types or not isinstance(root.get("manifests"), list)
            or len(root["manifests"]) != len(ROLES)):
        raise HelperImageError("helper archive OCI root index is invalid")
    observed_roles = set()
    for value in root["manifests"]:
        annotations = value.get("annotations") if isinstance(value, dict) else None
        tag = (annotations.get("io.containerd.image.name", annotations.get("org.opencontainers.image.ref.name"))
               if isinstance(annotations, dict) else None)
        role = next((role for role in ROLES if tag in {tags[role], "docker.io/library/" + tags[role]}), None)
        if role is None or role in observed_roles:
            raise HelperImageError("helper archive OCI root index does not bind unique prepared tags")
        observed_roles.add(role)
        ids = walk(value, role)
        if ids is not None:
            # Normal Docker inspect reports the imported top-level tag target.
            # Descendants and the untagged index.json wrapper are not IDs for
            # this tag, even when their configs happen to match.
            allowed[role].add(value["digest"])
    return {role: sorted(ids) for role, ids in allowed.items()}


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
