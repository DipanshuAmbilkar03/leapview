#!/usr/bin/env python3
"""Verify Docker-loaded helper images without assuming Docker image-ID semantics."""

import argparse
import hashlib
import json
import os
from pathlib import PurePosixPath
import re
import secrets
import signal
import subprocess
import sys
import tarfile
import time


ROLES = ("client", "browser")
SHA256 = re.compile(r"sha256:[0-9a-f]{64}\Z")
MAX_MEMBERS = 4096
MAX_MEMBER_BYTES = 2 * 1024**2
MAX_METADATA_BYTES = 8 * 1024**2
MAX_ARCHIVE_BYTES = 12 * 1024**3
DOCKER_SAVE_TIMEOUT_SECONDS = 1800
DOCKER_SAVE_WAIT_SECONDS = 30
DOCKER_INSPECT_TIMEOUT_SECONDS = 60
DOCKER_CREATE_TIMEOUT_SECONDS = 1800
DOCKER_REMOVE_TIMEOUT_SECONDS = 30
# An unknown engine ID first incurs two inspections, then the existing full
# export/inspection path. Application journey deadlines remain independent.
PROBE_TIMEOUT_SECONDS = DOCKER_SAVE_TIMEOUT_SECONDS + DOCKER_SAVE_WAIT_SECONDS + 4 * DOCKER_INSPECT_TIMEOUT_SECONDS + 30


class HelperImageProbeError(ValueError):
    pass


class _BoundedReader:
    """Count physical bytes read from Docker while keeping pipe reads bounded."""

    def __init__(self, stream, limit):
        self.stream = stream
        self.limit = limit
        self.total = 0

    def read(self, size=-1):
        if size is None or size < 0:
            size = 64 * 1024
        remaining = self.limit - self.total
        data = self.stream.read(min(size, remaining + 1))
        self.total += len(data)
        if self.total > self.limit:
            raise HelperImageProbeError("Docker image-save stream exceeds its byte limit")
        return data


def _no_duplicate_json_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise HelperImageProbeError("Docker image-save JSON contains duplicate keys")
        result[key] = value
    return result


def _read_member(archive, member, limit):
    if not member.isfile() or member.size < 0 or member.size > limit:
        raise HelperImageProbeError("Docker image-save metadata is not a bounded regular file")
    source = archive.extractfile(member)
    if source is None:
        raise HelperImageProbeError("Docker image-save metadata is missing")
    data = source.read(limit + 1)
    if len(data) != member.size or len(data) > limit:
        raise HelperImageProbeError("Docker image-save metadata length is invalid")
    return data


def _validate_manifest(raw, expected_tags, expected_config_digests):
    try:
        manifest = json.loads(raw, object_pairs_hook=_no_duplicate_json_keys)
    except (UnicodeDecodeError, json.JSONDecodeError, HelperImageProbeError) as exc:
        raise HelperImageProbeError("Docker image-save manifest is invalid") from exc
    if not isinstance(manifest, list) or len(manifest) != len(ROLES):
        raise HelperImageProbeError("Docker image-save manifest has an unexpected image count")
    tags_to_roles = {expected_tags[role]: role for role in ROLES}
    if len(tags_to_roles) != len(ROLES):
        raise HelperImageProbeError("prepared helper tags are not unique")
    configs = {}
    digest_to_role = {expected_config_digests[role].removeprefix("sha256:"): role for role in ROLES}
    for entry in manifest:
        if not isinstance(entry, dict):
            raise HelperImageProbeError("Docker image-save manifest entry is invalid")
        tags = entry.get("RepoTags")
        config = entry.get("Config")
        if not isinstance(tags, list) or len(tags) != 1 or tags[0] not in tags_to_roles:
            raise HelperImageProbeError("Docker image-save manifest has an unexpected tag")
        role = tags_to_roles[tags[0]]
        path = PurePosixPath(config) if isinstance(config, str) else None
        basename = path.name if path is not None else ""
        if (path is None or not config or path.is_absolute() or path.as_posix() != config
                or any(part in ("", ".", "..") for part in path.parts) or "\\" in config
                or config in configs or basename not in {
                    expected_config_digests[role].removeprefix("sha256:"),
                    expected_config_digests[role].removeprefix("sha256:") + ".json",
                } or digest_to_role.get(basename.removesuffix(".json")) != role):
            raise HelperImageProbeError("Docker image-save config reference is invalid")
        configs[config] = role
    if set(configs.values()) != set(ROLES) or len(configs) != len(ROLES):
        raise HelperImageProbeError("Docker image-save manifest does not bind both helper roles")
    return configs


def verify_saved_archive(stream, *, expected_tags, expected_config_digests, platform,
                         max_archive_bytes=MAX_ARCHIVE_BYTES):
    """Stream one bounded `docker image save` archive and verify tag-to-config bindings."""
    if (set(expected_tags) != set(ROLES) or set(expected_config_digests) != set(ROLES)
            or platform not in {"linux/amd64", "linux/arm64"}
            or any(not isinstance(expected_tags[role], str) or not expected_tags[role]
                   for role in ROLES)
            or any(not isinstance(expected_config_digests[role], str)
                   or SHA256.fullmatch(expected_config_digests[role]) is None for role in ROLES)
            or len(set(expected_config_digests.values())) != len(ROLES)):
        raise HelperImageProbeError("prepared helper image bindings are invalid")

    manifest_configs = None
    observed_configs = {}
    total_bytes = 0
    metadata_bytes = 0
    member_names = set()
    member_count = 0
    expected_configs = {
        (expected_config_digests[role].removeprefix("sha256:"), role) for role in ROLES
    }
    expected_configs |= {(name + ".json", role) for name, role in expected_configs}
    bounded_stream = _BoundedReader(stream, max_archive_bytes)
    try:
        archive = tarfile.open(fileobj=bounded_stream, mode="r|")
    except (tarfile.TarError, OSError) as exc:
        raise HelperImageProbeError("Docker image-save stream is not a readable archive") from exc
    with archive:
        for member in archive:
            member_count += 1
            total_bytes += member.size
            raw_name = member.name[:-1] if member.isdir() and member.name.endswith("/") else member.name
            name = PurePosixPath(raw_name)
            if (member_count > MAX_MEMBERS or member.size < 0 or total_bytes > max_archive_bytes
                    or not raw_name or name.is_absolute() or name.as_posix() != raw_name
                    or any(part in ("", ".", "..") for part in name.parts) or "\\" in member.name
                    or raw_name in member_names or not (member.isfile() or member.isdir())
                    or (member.isdir() and member.size != 0)
                    or (member.isfile() and member.name != raw_name)):
                raise HelperImageProbeError("Docker image-save archive is unsafe or oversized")
            member_names.add(raw_name)
            if member.isdir():
                continue
            if raw_name == "manifest.json":
                raw = _read_member(archive, member, MAX_MEMBER_BYTES)
                metadata_bytes += len(raw)
                if metadata_bytes > MAX_METADATA_BYTES or manifest_configs is not None:
                    raise HelperImageProbeError("Docker image-save manifest is duplicated or oversized")
                manifest_configs = _validate_manifest(raw, expected_tags, expected_config_digests)
                continue
            basename = name.name
            candidate_role = next((role for expected_name, role in expected_configs if basename == expected_name), None)
            manifest_role = manifest_configs.get(raw_name) if manifest_configs is not None else None
            role = manifest_role or candidate_role
            if role is None:
                continue
            if candidate_role is not None and candidate_role != role:
                raise HelperImageProbeError("Docker image-save config path binds to the wrong helper role")
            if role in observed_configs:
                raise HelperImageProbeError("Docker image-save archive repeats helper config metadata")
            raw = _read_member(archive, member, MAX_MEMBER_BYTES)
            metadata_bytes += len(raw)
            if metadata_bytes > MAX_METADATA_BYTES:
                raise HelperImageProbeError("Docker image-save config metadata is oversized")
            try:
                config = json.loads(raw, object_pairs_hook=_no_duplicate_json_keys)
            except (UnicodeDecodeError, json.JSONDecodeError, HelperImageProbeError) as exc:
                raise HelperImageProbeError("Docker image-save config is invalid") from exc
            if not isinstance(config, dict):
                raise HelperImageProbeError("Docker image-save config is invalid")
            observed_configs[role] = {
                "path": raw_name,
                "configDigest": "sha256:" + hashlib.sha256(raw).hexdigest(),
                "os": config.get("os"),
                "architecture": config.get("architecture"),
            }
        # Docker emits normal tar end blocks and sometimes padding after the
        # last parsed entry. Drain it, but still account for every physical byte
        # so a writer cannot block the subsequent wait or append an unbounded tail.
        while bounded_stream.read(64 * 1024):
            pass
    if (manifest_configs is None or set(observed_configs) != set(ROLES)
            or {role: observed_configs[role]["path"] for role in ROLES} != {
                role: path for path, role in manifest_configs.items()
            }):
        raise HelperImageProbeError("Docker image-save archive omitted helper config metadata")
    expected_os, expected_arch = platform.split("/")
    for role in ROLES:
        observed = observed_configs[role]
        if (observed["configDigest"] != expected_config_digests[role]
                or observed["os"] != expected_os or observed["architecture"] != expected_arch):
            raise HelperImageProbeError("loaded helper image content or platform differs from preparation")
    return observed_configs


def inspect_loaded_helpers(*, expected_tags, expected_config_digests, platform,
                           docker_bin="docker", max_archive_bytes=MAX_ARCHIVE_BYTES,
                           allowed_local_image_ids=None):
    """Compare one saved archive with immutable IDs reported by the active Docker engine."""
    if allowed_local_image_ids is not None:
        if (not isinstance(allowed_local_image_ids, dict) or set(allowed_local_image_ids) != set(ROLES)
                or not isinstance(expected_tags, dict) or set(expected_tags) != set(ROLES)
                or not isinstance(expected_config_digests, dict) or set(expected_config_digests) != set(ROLES)
                or platform not in {"linux/amd64", "linux/arm64"}
                or any(not isinstance(expected_tags[role], str) or not expected_tags[role] for role in ROLES)
                or len(set(expected_tags.values())) != len(ROLES)
                or any(not isinstance(expected_config_digests[role], str)
                       or SHA256.fullmatch(expected_config_digests[role]) is None for role in ROLES)
                or len(set(expected_config_digests.values())) != len(ROLES)):
            raise HelperImageProbeError("authenticated helper ID bindings are invalid")
        for role in ROLES:
            ids = allowed_local_image_ids[role]
            if (not isinstance(ids, list) or not 1 <= len(ids) <= 2
                    or any(not isinstance(value, str) or SHA256.fullmatch(value) is None for value in ids)
                    or len(set(ids)) != len(ids) or expected_config_digests[role] not in ids):
                raise HelperImageProbeError("authenticated helper ID allowlist is invalid")
        if set(allowed_local_image_ids["client"]) & set(allowed_local_image_ids["browser"]):
            raise HelperImageProbeError("authenticated helper role IDs overlap")
        images = _inspect_helpers(expected_tags, expected_config_digests, platform, docker_bin)
        if all(image["localImageID"] in allowed_local_image_ids[role] for role, image in images.items()):
            deadline = time.monotonic() + DOCKER_CREATE_TIMEOUT_SECONDS
            for image in images.values():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise HelperImageProbeError("Docker helper materialization exceeded its shared timeout")
                _materialize_helper(image["localImageID"], platform, docker_bin, timeout=remaining)
            return {"schemaVersion": 1, "archiveConfigDigests": dict(expected_config_digests),
                    "platform": platform, "images": images}
    process = subprocess.Popen(
        ["timeout", "--kill-after=10s", str(DOCKER_SAVE_TIMEOUT_SECONDS) + "s",
         docker_bin, "image", "save", *(expected_tags[role] for role in ROLES)],
        stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    try:
        configs = verify_saved_archive(
            process.stdout, expected_tags=expected_tags,
            expected_config_digests=expected_config_digests, platform=platform,
            max_archive_bytes=max_archive_bytes,
        )
        status = process.wait(timeout=DOCKER_SAVE_WAIT_SECONDS)
        if status != 0:
            raise HelperImageProbeError("Docker image save failed")
    except BaseException as exc:
        if process.poll() is None:
            os_killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os_killpg(process.pid, signal.SIGKILL)
                process.wait()
        if isinstance(exc, tarfile.TarError):
            raise HelperImageProbeError("Docker image-save stream is not a valid tar archive") from exc
        raise
    finally:
        if process.stdout is not None:
            process.stdout.close()

    images = _inspect_helpers(expected_tags, expected_config_digests, platform, docker_bin)
    return {"schemaVersion": 1, "archiveConfigDigests": dict(expected_config_digests),
            "platform": platform, "images": images}


def _inspect_helpers(expected_tags, expected_config_digests, platform, docker_bin):
    images = {}
    for role in ROLES:
        reference = expected_tags[role]
        try:
            inspected = subprocess.run(
                [docker_bin, "image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", reference],
                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                text=True, check=True, timeout=DOCKER_INSPECT_TIMEOUT_SECONDS,
            ).stdout.strip()
        except (OSError, subprocess.SubprocessError) as exc:
            raise HelperImageProbeError("Docker could not inspect a loaded helper tag") from exc
        fields = inspected.split()
        if (len(fields) != 2 or SHA256.fullmatch(fields[0]) is None or fields[1] != platform):
            raise HelperImageProbeError("Docker loaded helper ID or platform is invalid")
        images[role] = {
            "reference": reference,
            "configDigest": expected_config_digests[role],
            "localImageID": fields[0],
            "platform": platform,
        }
    return images


def _materialize_helper(image_id, platform, docker_bin, *, timeout):
    """Prepare the immutable rootfs without starting a helper or contacting a registry."""
    name = "leapview-helper-probe-" + secrets.token_hex(16)
    failure = None
    try:
        try:
            result = subprocess.run(
                [docker_bin, "container", "create", "--pull=never", "--platform", platform, "--name", name,
                 image_id, "/bin/true"],
                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                text=True, check=True, timeout=timeout,
            )
            if re.fullmatch(r"[0-9a-f]{64}", result.stdout.strip()) is None:
                failure = HelperImageProbeError("Docker helper availability probe returned an invalid container ID")
        except (OSError, subprocess.SubprocessError):
            failure = HelperImageProbeError("Docker could not materialize an authenticated helper image")
    finally:
        # A timeout can leave daemon-side creation pending. Removal failure
        # always rejects; the owning launcher tears down the disposable guest.
        try:
            subprocess.run(
                [docker_bin, "container", "rm", "--force", "--volumes", name],
                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                check=True, timeout=DOCKER_REMOVE_TIMEOUT_SECONDS,
            )
        except (OSError, subprocess.SubprocessError):
            raise HelperImageProbeError("Docker helper availability probe cleanup failed") from failure
    if failure is not None:
        raise failure


def validate_probe_result(result, *, expected_tags, expected_config_digests, platform):
    expected_keys = {"schemaVersion", "archiveConfigDigests", "platform", "images"}
    if (not isinstance(result, dict) or set(result) != expected_keys
            or type(result["schemaVersion"]) is not int or result["schemaVersion"] != 1
            or result["archiveConfigDigests"] != expected_config_digests
            or result["platform"] != platform or not isinstance(result["images"], dict)
            or set(result["images"]) != set(ROLES)):
        raise HelperImageProbeError("loaded helper probe result has an unsupported identity schema")
    for role in ROLES:
        image = result["images"][role]
        if (not isinstance(image, dict)
                or set(image) != {"reference", "configDigest", "localImageID", "platform"}
                or image["reference"] != expected_tags[role]
                or image["configDigest"] != expected_config_digests[role]
                or not isinstance(image["localImageID"], str)
                or SHA256.fullmatch(image["localImageID"]) is None
                or image["platform"] != platform):
            raise HelperImageProbeError("loaded helper role identity is not bound to its prepared image")
    return result


def validate_loaded_identity(identity, *, expected_tags, expected_config_digests,
                             archive_sha256, probe_sha256, platform):
    expected_keys = {"schemaVersion", "scope", "archiveSHA256", "probeSHA256", "platform", "images"}
    if (not isinstance(identity, dict) or set(identity) != expected_keys
            or type(identity["schemaVersion"]) is not int or identity["schemaVersion"] != 1
            or identity["scope"] != "docker-loaded-compose-authoring-helpers"
            or identity["archiveSHA256"] != archive_sha256
            or identity["probeSHA256"] != probe_sha256 or SHA256.fullmatch(probe_sha256 or "") is None
            or identity["platform"] != platform):
        raise HelperImageProbeError("loaded helper identity receipt has an unsupported schema or binding")
    validate_probe_result(
        {"schemaVersion": 1, "archiveConfigDigests": expected_config_digests,
         "platform": platform, "images": identity["images"]},
        expected_tags=expected_tags, expected_config_digests=expected_config_digests,
        platform=platform,
    )
    return identity


def os_killpg(pid, sig):
    try:
        os.killpg(pid, sig)
    except ProcessLookupError:
        pass


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("--spec", required=True, help="JSON helper tag and config bindings")
    parser.add_argument("--docker-bin", default="docker")
    parser.add_argument("--max-archive-bytes", type=int, default=MAX_ARCHIVE_BYTES)
    args = parser.parse_args(argv)
    if not 0 < args.max_archive_bytes <= MAX_ARCHIVE_BYTES:
        raise SystemExit("invalid archive size bound")
    try:
        spec = json.loads(args.spec)
        required = {"tags", "configDigests", "platform"}
        if not isinstance(spec, dict) or set(spec) not in (required, required | {"allowedLocalImageIDs"}):
            raise HelperImageProbeError("prepared helper probe specification has an unsupported schema")
        if "allowedLocalImageIDs" in spec and not isinstance(spec["allowedLocalImageIDs"], dict):
            raise HelperImageProbeError("authenticated helper ID allowlist is invalid")
        result = inspect_loaded_helpers(
            expected_tags=spec["tags"], expected_config_digests=spec["configDigests"],
            platform=spec["platform"], docker_bin=args.docker_bin,
            max_archive_bytes=args.max_archive_bytes,
            allowed_local_image_ids=spec.get("allowedLocalImageIDs"),
        )
    except (HelperImageProbeError, KeyError, TypeError, ValueError) as exc:
        raise SystemExit(str(exc)) from exc
    sys.stdout.write(json.dumps(result, sort_keys=True, separators=(",", ":")) + "\n")


if __name__ == "__main__":
    main()
