"""The dates the docs sitemap carries, and the sitemap it writes.

Run with: python -m unittest discover -s hooks -p '*_test.py'
"""

import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import docs  # noqa: E402


def commit(repo: Path, date: str, message: str, files: dict[str, str]) -> None:
    for name, text in files.items():
        path = repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
    env = {
        **os.environ,
        "GIT_AUTHOR_NAME": "t",
        "GIT_AUTHOR_EMAIL": "t@example.com",
        "GIT_COMMITTER_NAME": "t",
        "GIT_COMMITTER_EMAIL": "t@example.com",
        "GIT_AUTHOR_DATE": date,
        "GIT_COMMITTER_DATE": date,
    }
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True, env=env)
    subprocess.run(["git", "commit", "-q", "-m", message], cwd=repo, check=True, env=env)


class GitDates(unittest.TestCase):
    def setUp(self):
        self.repo = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.repo, ignore_errors=True)
        subprocess.run(["git", "init", "-q", "-b", "main"], cwd=self.repo, check=True)
        commit(self.repo, "2026-01-02T10:00:00+00:00", "a", {"docs/a.md": "one"})
        commit(self.repo, "2026-02-03T10:00:00+00:00", "b", {"docs/guides/b.md": "one"})
        commit(self.repo, "2026-03-04T10:00:00+00:00", "a again", {"docs/a.md": "two"})
        commit(self.repo, "2026-05-05T10:00:00+00:00", "readme", {"README.md": "outside the docs"})

    def test_a_page_is_dated_by_its_last_edit_not_its_first(self):
        dates = docs._git_dates(str(self.repo / "docs"))
        self.assertEqual(dates["a.md"], "2026-03-04T10:00:00Z")

    def test_paths_are_relative_to_the_docs_directory(self):
        dates = docs._git_dates(str(self.repo / "docs"))
        self.assertEqual(dates["guides/b.md"], "2026-02-03T10:00:00Z")

    def test_a_change_outside_the_docs_does_not_date_anything(self):
        dates = docs._git_dates(str(self.repo / "docs"))
        self.assertNotIn("README.md", dates)
        self.assertEqual(set(dates), {"a.md", "guides/b.md"})

    def test_two_edits_on_the_same_day_get_different_stamps(self):
        # A docs deploy runs on every merge, and a page is often edited more than
        # once in a day. A date alone would give both versions the same lastmod and
        # the second deploy would tell engines nothing.
        commit(self.repo, "2026-06-01T09:00:00+00:00", "morning", {"docs/c.md": "one"})
        morning = docs._git_dates(str(self.repo / "docs"))["c.md"]
        commit(self.repo, "2026-06-01T10:00:00+00:00", "later", {"docs/c.md": "two"})
        later = docs._git_dates(str(self.repo / "docs"))["c.md"]
        self.assertNotEqual(morning, later)

    def test_a_stamp_is_in_utc_whatever_zone_the_commit_was_made_in(self):
        # The same moment must read the same, or a contributor's time zone would
        # look like an edit.
        commit(self.repo, "2026-06-02T12:00:00+02:00", "abroad", {"docs/d.md": "one"})
        self.assertEqual(docs._git_dates(str(self.repo / "docs"))["d.md"], "2026-06-02T10:00:00Z")

    def test_a_directory_with_no_history_gives_no_dates_rather_than_failing_the_build(self):
        with tempfile.TemporaryDirectory() as bare:
            self.assertEqual(docs._git_dates(bare), {})


class OnFiles(unittest.TestCase):
    def files(self, *pairs):
        pages = [SimpleNamespace(src_uri=src, dest_uri=dest) for src, dest in pairs]
        return SimpleNamespace(documentation_pages=lambda: pages)

    def test_a_page_is_dated_under_the_file_mkdocs_writes_for_it(self):
        repo = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, repo, ignore_errors=True)
        subprocess.run(["git", "init", "-q", "-b", "main"], cwd=repo, check=True)
        commit(repo, "2026-03-04T10:00:00+00:00", "a", {"docs/documentation.md": "x", "docs/new.md": "x"})
        commit(repo, "2026-04-05T10:00:00+00:00", "b", {"docs/documentation.md": "y"})
        files = self.files(
            ("documentation.md", "documentation/index.html"),
            ("new.md", "new/index.html"),
            ("uncommitted.md", "uncommitted/index.html"),
        )
        docs.on_files(files, {"docs_dir": str(repo / "docs")})
        self.assertEqual(
            docs._lastmod,
            {"documentation/index.html": "2026-04-05T10:00:00Z", "new/index.html": "2026-03-04T10:00:00Z"},
        )

    def test_the_dates_of_one_build_do_not_leak_into_the_next(self):
        docs._lastmod = {"stale/index.html": "2020-01-01"}
        with tempfile.TemporaryDirectory() as bare:
            docs.on_files(self.files(), {"docs_dir": bare})
        self.assertEqual(docs._lastmod, {})


class Sitemap(unittest.TestCase):
    def written(self, pages):
        with tempfile.TemporaryDirectory() as site:
            docs.write_sitemap(site, pages)
            return Path(site, "sitemap.xml").read_text(encoding="utf-8")

    def test_a_dated_page_carries_its_last_edit(self):
        xml = self.written([{"url": "https://reet.sh/a/", "lastmod": "2026-03-04T10:00:00Z"}])
        self.assertIn("<loc>https://reet.sh/a/</loc>\n    <lastmod>2026-03-04T10:00:00Z</lastmod>", xml)

    def test_an_undated_page_is_listed_without_inventing_a_date(self):
        xml = self.written([{"url": "https://reet.sh/a/", "lastmod": ""}])
        self.assertIn("<loc>https://reet.sh/a/</loc>", xml)
        self.assertNotIn("lastmod", xml)


if __name__ == "__main__":
    unittest.main()
