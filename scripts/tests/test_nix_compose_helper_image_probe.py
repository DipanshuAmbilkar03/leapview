import hashlib
import io
import json
from pathlib import Path
import sys
import tarfile
import unittest
import subprocess
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from nix_compose_helper_image_probe import (
    HelperImageProbeError,
    inspect_loaded_helpers,
    verify_saved_archive,
)


TAGS = {
    "client": "leapview-qualification-client:protected-amd64",
    "browser": "leapview-qualification-browser:protected-amd64",
}
CONFIGS = {
    "client": json.dumps({"os": "linux", "architecture": "amd64", "config": {"Labels": {"role": "client"}},
                           "rootfs": {"type": "layers", "diff_ids": []}},
                         sort_keys=True, separators=(",", ":")).encode(),
    "browser": json.dumps({"os": "linux", "architecture": "amd64", "config": {"Labels": {"role": "browser"}},
                            "rootfs": {"type": "layers", "diff_ids": []}},
                          sort_keys=True, separators=(",", ":")).encode(),
}
CONFIG_DIGESTS = {role: "sha256:" + hashlib.sha256(raw).hexdigest() for role, raw in CONFIGS.items()}


def _archive(*, tags=None, configs=None, manifest_first=False, classic_config_names=False,
             duplicate_config_member=False, unsafe_config_reference=False):
    tags = tags or TAGS
    configs = configs or CONFIGS
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w") as archive:
        entries = []
        for role in ("client", "browser"):
            config_path = CONFIG_DIGESTS[role].split(":", 1)[1] + ".json"
            if not classic_config_names:
                config_path = "blobs/sha256/" + CONFIG_DIGESTS[role].split(":", 1)[1]
            if unsafe_config_reference and role == "client":
                config_path = "../" + config_path
            entries.append({"Config": config_path, "RepoTags": [tags[role]], "Layers": ["layer.tar"]})
        manifest = json.dumps(entries, sort_keys=True, separators=(",", ":")).encode()
        config_entries = [((CONFIG_DIGESTS[role].split(":", 1)[1] + ".json") if classic_config_names
                           else ("blobs/sha256/" + CONFIG_DIGESTS[role].split(":", 1)[1]), configs[role])
                          for role in ("client", "browser")]
        if manifest_first:
            entries_in_order = [("manifest.json", manifest), *config_entries]
        else:
            entries_in_order = [*config_entries, ("manifest.json", manifest)]
        if duplicate_config_member:
            entries_in_order.append(config_entries[0])
        for name, data in entries_in_order:
            member = tarfile.TarInfo(name)
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))
    output.seek(0)
    return output


class HelperImageProbeTests(unittest.TestCase):
    def test_authenticated_ids_avoid_export_and_materialize_each_immutable_image(self):
        ids = {role: [CONFIG_DIGESTS[role], "sha256:" + letter * 64]
               for role, letter in (("client", "d"), ("browser", "e"))}
        results = [SimpleNamespace(stdout=ids[role][1] + " linux/amd64\n") for role in ("client", "browser")]
        results += [SimpleNamespace(stdout="a" * 64 + "\n"), SimpleNamespace(stdout="")] * 2
        with patch("nix_compose_helper_image_probe.subprocess.Popen") as popen, \
             patch("nix_compose_helper_image_probe.subprocess.run", side_effect=results) as run:
            observed = inspect_loaded_helpers(expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS,
                platform="linux/amd64", allowed_local_image_ids=ids)
        popen.assert_not_called()
        creates = [call.args[0] for call in run.call_args_list if "create" in call.args[0]]
        self.assertEqual(len(creates), 2)
        for role, command in zip(("client", "browser"), creates):
            self.assertIn("--pull=never", command)
            self.assertEqual(command[command.index("--platform") + 1], "linux/amd64")
            self.assertEqual(command[-2:], [ids[role][1], "/bin/true"])
            self.assertEqual(observed["images"][role]["localImageID"], ids[role][1])
        removals = [call.args[0] for call in run.call_args_list if "rm" in call.args[0]]
        self.assertEqual(len(removals), 2)
        self.assertTrue(all("--volumes" in command for command in removals))

    def test_unknown_immutable_id_falls_back_to_complete_export_validation(self):
        ids = {role: [CONFIG_DIGESTS[role]] for role in ("client", "browser")}
        results = [SimpleNamespace(stdout="sha256:" + letter * 64 + " linux/amd64\n") for letter in "dede"]
        process = SimpleNamespace(stdout=_archive(), wait=lambda **_: 0, poll=lambda: 0)
        with patch("nix_compose_helper_image_probe.subprocess.Popen", return_value=process) as popen, \
             patch("nix_compose_helper_image_probe.subprocess.run", side_effect=results):
            inspect_loaded_helpers(expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS,
                platform="linux/amd64", allowed_local_image_ids=ids)
        popen.assert_called_once()

    def test_fast_path_rejects_invalid_allowlists_before_docker_calls(self):
        valid = {role: [CONFIG_DIGESTS[role]] for role in ("client", "browser")}
        for invalid in ({}, valid | {"extra": []}, valid | {"client": ["latest"]},
                        valid | {"client": ["sha256:" + "0" * 64]},
                        valid | {"client": [CONFIG_DIGESTS["client"]] * 2}):
            with self.subTest(invalid=invalid), patch("nix_compose_helper_image_probe.subprocess.run") as run:
                with self.assertRaises(HelperImageProbeError):
                    inspect_loaded_helpers(expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS,
                        platform="linux/amd64", allowed_local_image_ids=invalid)
                run.assert_not_called()

    def test_fast_path_availability_timeout_still_removes_owned_container_and_cleanup_failure_rejects(self):
        ids = {role: [CONFIG_DIGESTS[role]] for role in ("client", "browser")}
        inspected = [SimpleNamespace(stdout=CONFIG_DIGESTS[role] + " linux/amd64\n") for role in ("client", "browser")]
        for failure in (subprocess.TimeoutExpired("docker create", 120), SimpleNamespace(stdout="a" * 64 + "\n")):
            results = inspected + [failure, subprocess.CalledProcessError(1, "docker rm")]
            with self.subTest(failure=failure), patch("nix_compose_helper_image_probe.subprocess.run", side_effect=results) as run:
                with self.assertRaises(HelperImageProbeError):
                    inspect_loaded_helpers(expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS,
                        platform="linux/amd64", allowed_local_image_ids=ids)
                self.assertIn("rm", run.call_args_list[-1].args[0])

    def test_fast_path_materialization_uses_one_shared_deadline_for_both_roles(self):
        ids = {role: [CONFIG_DIGESTS[role]] for role in ("client", "browser")}
        results = [SimpleNamespace(stdout=CONFIG_DIGESTS[role] + " linux/amd64\n") for role in ("client", "browser")]
        results += [SimpleNamespace(stdout="a" * 64 + "\n"), SimpleNamespace(stdout="")]
        with patch("nix_compose_helper_image_probe.time.monotonic", side_effect=[0, 0, 1801]), \
             patch("nix_compose_helper_image_probe.subprocess.run", side_effect=results) as run:
            with self.assertRaisesRegex(HelperImageProbeError, "shared timeout"):
                inspect_loaded_helpers(expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS,
                    platform="linux/amd64", allowed_local_image_ids=ids)
        self.assertEqual(run.call_count, 4)

    def test_one_docker_save_binds_tags_to_prepared_config_and_accepts_engine29_ids(self):
        archive = _archive()
        client_manifest_id = "sha256:" + "d" * 64
        browser_manifest_id = "sha256:" + "e" * 64
        process = SimpleNamespace(stdout=archive, wait=lambda **_: 0, poll=lambda: 0)
        with patch("nix_compose_helper_image_probe.subprocess.Popen", return_value=process) as popen, \
             patch("nix_compose_helper_image_probe.subprocess.run", side_effect=(
                 SimpleNamespace(stdout=client_manifest_id + " linux/amd64\n"),
                 SimpleNamespace(stdout=browser_manifest_id + " linux/amd64\n"),
             )) as run:
            observed = inspect_loaded_helpers(
                expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
            )

        popen.assert_called_once_with(
            ["timeout", "--kill-after=10s", "1800s", "docker", "image", "save", TAGS["client"], TAGS["browser"]],
            stdin=unittest.mock.ANY, stdout=unittest.mock.ANY, stderr=unittest.mock.ANY,
            start_new_session=True,
        )
        self.assertEqual(run.call_count, 2)
        self.assertEqual(run.call_args_list[0].args[0][-1], TAGS["client"])
        self.assertEqual(run.call_args_list[1].args[0][-1], TAGS["browser"])
        self.assertEqual(observed["images"]["client"]["configDigest"], CONFIG_DIGESTS["client"])
        self.assertEqual(observed["images"]["client"]["localImageID"], client_manifest_id)
        self.assertEqual(observed["images"]["browser"]["configDigest"], CONFIG_DIGESTS["browser"])
        self.assertEqual(observed["images"]["browser"]["localImageID"], browser_manifest_id)

    def test_saved_archive_rejects_a_config_or_platform_mismatch(self):
        changed = dict(CONFIGS)
        changed["browser"] = json.dumps({"os": "linux", "architecture": "arm64"}).encode()
        with self.assertRaisesRegex(HelperImageProbeError, "content or platform"):
            verify_saved_archive(
                _archive(configs=changed), expected_tags=TAGS,
                expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
            )

    def test_saved_archive_binds_configs_when_manifest_is_last(self):
        observed = verify_saved_archive(
            _archive(manifest_first=False), expected_tags=TAGS,
            expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
        )
        self.assertEqual(observed["client"]["configDigest"], CONFIG_DIGESTS["client"])
        self.assertEqual(observed["browser"]["configDigest"], CONFIG_DIGESTS["browser"])

    def test_saved_archive_binds_configs_when_manifest_is_first_with_classic_names(self):
        observed = verify_saved_archive(
            _archive(manifest_first=True, classic_config_names=True), expected_tags=TAGS,
            expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
        )
        self.assertEqual(observed["client"]["configDigest"], CONFIG_DIGESTS["client"])
        self.assertEqual(observed["browser"]["configDigest"], CONFIG_DIGESTS["browser"])

    def test_saved_archive_rejects_duplicate_members_and_unsafe_config_paths(self):
        for archive, expected in (
            (_archive(duplicate_config_member=True), "unsafe or oversized"),
            (_archive(unsafe_config_reference=True), "config reference is invalid"),
        ):
            with self.subTest(expected=expected), self.assertRaisesRegex(HelperImageProbeError, expected):
                verify_saved_archive(
                    archive, expected_tags=TAGS,
                    expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
                )

    def test_saved_archive_is_bounded(self):
        with self.assertRaisesRegex(HelperImageProbeError, "byte limit"):
            verify_saved_archive(
                _archive(), expected_tags=TAGS,
                expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
                max_archive_bytes=16,
            )

    def test_loaded_helper_requires_an_immutable_id_and_native_platform(self):
        process = SimpleNamespace(stdout=_archive(), wait=lambda **_: 0, poll=lambda: 0)
        with patch("nix_compose_helper_image_probe.subprocess.Popen", return_value=process), \
             patch("nix_compose_helper_image_probe.subprocess.run", return_value=SimpleNamespace(stdout="latest linux/amd64\n")):
            with self.assertRaisesRegex(HelperImageProbeError, "ID or platform is invalid"):
                inspect_loaded_helpers(
                    expected_tags=TAGS, expected_config_digests=CONFIG_DIGESTS, platform="linux/amd64",
                )


if __name__ == "__main__":
    unittest.main()
