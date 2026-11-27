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

    def test_gitee_only_attaches_the_files_missing_from_the_release(self):
        with tempfile.TemporaryDirectory() as temporary:
            paths = [Path(temporary) / name for name in ("present.zip", "missing.zip", "SHA256SUMS")]
            for path in paths:
                path.write_bytes(path.name.encode())
            release = {"id": 7, "assets": [{"name": paths[0].name}]}
            events = []

            def api_call(_token, path, *args, **kwargs):
                events.append(path)
                return release

            with patch.object(publish, "assets", return_value=paths), patch.object(
                publish, "gitee_push", side_effect=lambda ref: events.append(ref)
            ), patch.object(publish, "gitee_call", side_effect=api_call) as api:
                publish.gitee("v0.0.1", "commit", "token")
            uploads = api.call_args_list[1:]
            self.assertEqual(len(uploads), 2)
            self.assertTrue(all(call.args[1] == "/releases/7/attach_files" for call in uploads))
            self.assertCountEqual([call.args[2].split(b'filename="')[1].split(b'"')[0] for call in uploads],
                                  [b"missing.zip", b"SHA256SUMS"])
            self.assertEqual(events[0], "refs/tags/v0.0.1")
            self.assertEqual(events[-1], "commit:refs/heads/main")

    def test_gitee_releases_exact_commit_without_advancing_main_on_upload_failure(self):
        with tempfile.TemporaryDirectory() as temporary:
            payload = Path(temporary) / "binary.zip"
            payload.write_bytes(b"archive")
            replies = [None, {"id": 7}, SystemExit("attach failed")]
            with patch.object(publish, "assets", return_value=[payload]), patch.object(publish, "gitee_push") as push, patch.object(
                publish, "gitee_call", side_effect=replies
            ) as api, patch.object(publish, "OUT", Path(temporary)):
                (Path(temporary) / "notes.md").write_text("Release notes", encoding="utf-8")
                with self.assertRaisesRegex(SystemExit, "attach failed"):
                    publish.gitee("v0.0.1", "prepared-commit", "token")
            push.assert_called_once_with("refs/tags/v0.0.1")
            self.assertEqual(json.loads(api.call_args_list[1].args[2])["target_commitish"], "prepared-commit")


if __name__ == "__main__":
    unittest.main()
