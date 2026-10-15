import hashlib
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import nix_compose_helper_images as helpers


class HelperArchiveTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.archive = Path(self.temporary.name) / "helper-images.tar.gz"
        self.tags = {role: f"leapview-qualification-{role}:test" for role in ("client", "browser")}

    def write_archive(self, *, architecture="amd64", extra=False, swapped=False, unsafe=False):
        manifest, files, expected = [], {}, {}
        for role, tag in self.tags.items():
            config = json.dumps({"architecture": architecture, "os": "linux", "config": {"Labels": {"role": role}},
                                 "rootfs": {"type": "layers", "diff_ids": ["sha256:" + "a" * 64]}}).encode()
            digest = hashlib.sha256(config).hexdigest()
            name = "blobs/sha256/" + digest
            files[name] = config
            manifest.append({"Config": name, "RepoTags": [tag], "Layers": ["layer.tar"]})
            expected[role] = "sha256:" + digest
        if extra:
            manifest.append(dict(manifest[0], RepoTags=["unexpected:latest"]))
        if swapped:
            manifest[0]["RepoTags"] = ["unexpected:latest"]
        files["manifest.json"] = json.dumps(manifest).encode()
        files["layer.tar"] = b"fixture layer; loading is tested separately"
        if unsafe:
            files["../escape"] = b"unexpected"
        with tarfile.open(self.archive, "w:gz") as archive:
            for name, value in files.items():
                entry = tarfile.TarInfo(name)
                entry.size = len(value)
                archive.addfile(entry, io.BytesIO(value))
        return expected

    def test_uses_archive_configuration_digests_not_daemon_image_ids(self):
        expected = self.write_archive()
        self.assertEqual(helpers.archive_images(self.archive, self.tags, "linux/amd64"), expected)

    def test_rejects_wrong_architecture(self):
        self.write_archive(architecture="arm64")
        with self.assertRaisesRegex(helpers.HelperImageError, "platform"):
            helpers.archive_images(self.archive, self.tags, "linux/amd64")

    def test_rejects_additional_image(self):
        self.write_archive(extra=True)
        with self.assertRaises(helpers.HelperImageError):
            helpers.archive_images(self.archive, self.tags, "linux/amd64")

    def test_rejects_missing_expected_role(self):
        self.write_archive(swapped=True)
        with self.assertRaises(helpers.HelperImageError):
            helpers.archive_images(self.archive, self.tags, "linux/amd64")

    def test_rejects_unsafe_archive_path(self):
        self.write_archive(unsafe=True)
        with self.assertRaises(helpers.HelperImageError):
            helpers.archive_images(self.archive, self.tags, "linux/amd64")

    def test_rejects_archive_symlink(self):
        self.write_archive()
        alias = self.archive.with_name("alias.tar.gz")
        alias.symlink_to(self.archive)
        with self.assertRaises(helpers.HelperImageError):
            helpers.archive_images(alias, self.tags, "linux/amd64")

    def preparation(self):
        identity = {"source_revision": "1" * 40, "protected_revision": "2" * 40,
                    "image": "ghcr.io/flidai/leapview@sha256:" + "3" * 64, "platform": "linux/amd64",
                    "assets": {name: "sha256:" + "4" * 64 for name in helpers.ASSETS}}
        self.tags = helpers.image_tags(identity["protected_revision"], identity["platform"])
        images = self.write_archive()
        receipt = {"schemaVersion": 1, "scope": "native-compose-authoring-helpers", "result": "passed",
                   "sourceRevision": identity["source_revision"], "protectedRevision": identity["protected_revision"],
                   "image": identity["image"], "platform": identity["platform"],
                   "archiveSHA256": helpers.file_digest(self.archive), "assetsSHA256": identity["assets"], "images": images}
        (self.archive.parent / helpers.RECEIPT).write_text(json.dumps(receipt))
        return identity, receipt

    def test_preparation_binds_both_images_archive_assets_and_revisions(self):
        identity, receipt = self.preparation()
        self.assertEqual(helpers.verify_preparation(self.archive.parent, **identity), receipt)

    def test_rejects_changed_archive_bytes(self):
        identity, _ = self.preparation()
        with self.archive.open("ab") as archive:
            archive.write(b"changed")
        with self.assertRaisesRegex(helpers.HelperImageError, "checksum"):
            helpers.verify_preparation(self.archive.parent, **identity)

    def test_rejects_configuration_identity_substitution(self):
        identity, receipt = self.preparation()
        receipt["images"]["client"] = "sha256:" + "9" * 64
        (self.archive.parent / helpers.RECEIPT).write_text(json.dumps(receipt))
        with self.assertRaisesRegex(helpers.HelperImageError, "configuration identities"):
            helpers.verify_preparation(self.archive.parent, **identity)

    def test_rejects_wrong_qualification_identity(self):
        identity, _ = self.preparation()
        for field, value in [("source_revision", "5" * 40), ("protected_revision", "6" * 40),
                             ("image", "ghcr.io/flidai/leapview@sha256:" + "7" * 64),
                             ("platform", "linux/arm64"), ("assets", {name: "sha256:" + "8" * 64 for name in helpers.ASSETS})]:
            with self.subTest(field=field), self.assertRaises(helpers.HelperImageError):
                helpers.verify_preparation(self.archive.parent, **(identity | {field: value}))

    def test_rejects_boolean_schema_version(self):
        identity, receipt = self.preparation()
        receipt["schemaVersion"] = True
        with self.assertRaises(helpers.HelperImageError):
            helpers.validate_receipt(receipt, **identity)

    def write_oci_archive(self, *, nested=False, wrong_tag=False, wrong_config=False,
                          wrong_platform=False, tampered_blob=False, wrong_size=False,
                          ambiguous=False, unsafe=False, duplicate=False, tampered_layer=False,
                          conflicting_name=False, absent_layout=False, wrong_layout=False, small_binary_layers=0):
        files, saved, roots, configs, proven = {}, [], [], {}, {}
        encode = lambda value: json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
        def blob(value, media_type):
            raw = encode(value)
            digest = "sha256:" + hashlib.sha256(raw).hexdigest()
            files["blobs/sha256/" + digest[7:]] = raw
            return {"mediaType": media_type, "digest": digest, "size": len(raw)}
        for role, tag in self.tags.items():
            layer = (role + " layer").encode()
            layer_digest = "sha256:" + hashlib.sha256(layer).hexdigest()
            layer_path = "blobs/sha256/" + layer_digest[7:]
            files[layer_path] = layer
            config = {"os": "linux", "architecture": "arm64" if wrong_platform else "amd64",
                      "rootfs": {"type": "layers", "diff_ids": [layer_digest]}, "config": {"Labels": {"role": role}}}
            config_desc = blob(config, "application/vnd.oci.image.config.v1+json")
            configs[role] = config_desc["digest"]
            saved.append({"Config": "blobs/sha256/" + configs[role][7:], "RepoTags": [tag], "Layers": [layer_path]})
            if wrong_config and role == "browser":
                config_desc = dict(config_desc, digest=configs["client"])
            descriptor = blob({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
                               "config": config_desc, "layers": [{"mediaType": "application/vnd.oci.image.layer.v1.tar",
                               "digest": layer_digest, "size": len(layer)}]}, "application/vnd.oci.image.manifest.v1+json")
            proven[role] = [configs[role], descriptor["digest"]]
            if nested or ambiguous:
                child = dict(descriptor, platform={"os": "linux", "architecture": "amd64"})
                descriptor = blob({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
                                   "manifests": [child, child] if ambiguous else [child]}, "application/vnd.oci.image.index.v1+json")
                proven[role] = [configs[role], descriptor["digest"]]
            if tampered_blob and role == "browser":
                files["blobs/sha256/" + descriptor["digest"][7:]] += b" "
            if wrong_size and role == "browser":
                descriptor = dict(descriptor, size=descriptor["size"] + 1)
            roots.append(dict(descriptor, annotations={"io.containerd.image.name": "docker.io/library/" +
                         ("other:tag" if wrong_tag and role == "browser" else tag)}))
            if conflicting_name and role == "browser":
                roots[-1]["annotations"] = {"io.containerd.image.name": "other:tag", "org.opencontainers.image.ref.name": tag}
            if tampered_layer and role == "browser":
                files[layer_path] = b"different layer bytes"
        files["manifest.json"] = encode(saved)
        files["oci-layout"] = encode({"imageLayoutVersion": "1.0.0"})
        if absent_layout:
            del files["oci-layout"]
        if wrong_layout:
            files["oci-layout"] = encode({"imageLayoutVersion": "2.0.0"})
        files["index.json"] = encode({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": roots})
        unrelated = blob({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": []}, "application/vnd.oci.image.index.v1+json")["digest"]
        for index in range(small_binary_layers):
            raw = bytes([index + 1]) * (1024 * 1024)
            files["blobs/sha256/" + hashlib.sha256(raw).hexdigest()] = raw
        if unsafe:
            files["../late-escape"] = b"late unsafe entry"
        with tarfile.open(self.archive, "w:gz") as archive:
            for name, value in files.items():
                entry = tarfile.TarInfo(name)
                entry.size = len(value)
                archive.addfile(entry, io.BytesIO(value))
            if duplicate:
                entry = tarfile.TarInfo("manifest.json")
                entry.size = len(files["manifest.json"])
                archive.addfile(entry, io.BytesIO(files["manifest.json"]))
        return configs, proven, unrelated

    def archive_ids(self, configs, **kwargs):
        return helpers.archive_local_image_ids(self.archive, archive_sha256=kwargs.pop("archive_sha256", helpers.file_digest(self.archive)),
            tags=self.tags, config_digests=configs, platform="linux/amd64", **kwargs)

    def test_authenticated_archive_proves_only_tag_bound_config_manifest_and_index_ids(self):
        for nested in (False, True):
            with self.subTest(nested=nested):
                configs, proven, unrelated = self.write_oci_archive(nested=nested)
                observed = self.archive_ids(configs)
                self.assertEqual({role: set(ids) for role, ids in observed.items()},
                                 {role: set(ids) for role, ids in proven.items()})
                self.assertTrue(all(unrelated not in ids for ids in observed.values()))

    def test_archive_id_proof_rejects_unbound_or_corrupt_graphs_and_late_unsafe_members(self):
        for option in ("wrong_tag", "wrong_config", "wrong_platform", "tampered_blob", "wrong_size", "ambiguous", "unsafe", "duplicate",
                       "tampered_layer", "conflicting_name", "wrong_layout"):
            with self.subTest(option=option):
                configs, _, _ = self.write_oci_archive(**{option: True})
                with self.assertRaises(helpers.HelperImageError):
                    self.archive_ids(configs)

    def test_index_metadata_without_oci_layout_does_not_prove_imported_target_ids(self):
        configs, _, _ = self.write_oci_archive(absent_layout=True)
        self.assertEqual(self.archive_ids(configs), {role: [digest] for role, digest in configs.items()})

    def test_small_binary_layers_do_not_consume_json_metadata_budget(self):
        configs, proven, _ = self.write_oci_archive(small_binary_layers=9)
        self.assertEqual({role: set(ids) for role, ids in self.archive_ids(configs).items()},
                         {role: set(ids) for role, ids in proven.items()})

    def test_archive_id_proof_authenticates_same_complete_stream_and_rejects_symlinks(self):
        configs, _, _ = self.write_oci_archive()
        with self.assertRaisesRegex(helpers.HelperImageError, "checksum"):
            self.archive_ids(configs, archive_sha256="sha256:" + "0" * 64)
        alias = self.archive.with_name("alias.tar.gz")
        alias.symlink_to(self.archive)
        with self.assertRaises(helpers.HelperImageError):
            helpers.archive_local_image_ids(alias, archive_sha256=helpers.file_digest(self.archive),
                tags=self.tags, config_digests=configs, platform="linux/amd64")

    def test_classic_archive_proves_legacy_config_ids_without_inventing_manifest_ids(self):
        configs = self.write_archive()
        self.assertEqual(self.archive_ids(configs), {role: [digest] for role, digest in configs.items()})


if __name__ == "__main__":
    unittest.main()
