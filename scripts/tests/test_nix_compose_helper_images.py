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


if __name__ == "__main__":
    unittest.main()
