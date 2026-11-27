"""Verify local archive creation and release resumption without contacting either host."""

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import publish


class PublishTest(unittest.TestCase):
    def test_rebuild_has_identical_archives_and_ignores_other_zips(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            (output / "old.zip").write_bytes(b"older archive")

            def run(*command, **options):
                target = Path(command[command.index("-o") + 1])
                target.write_bytes(b"compiled binary")

            with patch.object(publish, "OUT", output), patch.object(publish, "TARGETS", ["windows/arm64"]), patch.object(
                publish, "run", side_effect=run
            ), patch.object(publish, "module_notices"):
                first = publish.build("0.0.1")[0].read_bytes()
                second = publish.build("0.0.1")[0].read_bytes()
                self.assertEqual(first, second)
                self.assertNotIn("old.zip", [path.name for path in publish.assets()])

    def test_existing_github_draft_uploads_missing_assets_and_is_published(self):
        paths = [Path("bailu-windows-amd64.zip"), Path("SHA256SUMS")]
        with patch.object(publish, "assets", return_value=paths), patch.object(publish, "run") as calls, patch.object(
            publish.subprocess, "run"
        ) as view:
            view.return_value.returncode = 0
            view.return_value.stdout = json.dumps({"isDraft": True, "assets": [{"name": "SHA256SUMS"}]})
            publish.github("v0.0.1")
        flags = ["--repo", publish.GITHUB_REPO]
        self.assertEqual(calls.call_args_list[0].args, ("gh", "release", "upload", "v0.0.1", paths[0].name, *flags))
        self.assertEqual(calls.call_args_list[1].args, ("gh", "release", "edit", "v0.0.1", "--draft=false", *flags))

    def test_existing_tag_cannot_be_reassigned(self):
        with patch.object(publish, "run", side_effect=["new-commit", "existing-commit"]) as git:
            with self.assertRaisesRegex(SystemExit, "already names a different commit"):
                publish.ensure_tag("v0.0.1")
        self.assertEqual(git.call_count, 2)


if __name__ == "__main__":
    unittest.main()
