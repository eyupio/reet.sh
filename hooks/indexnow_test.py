"""What the docs workflow tells search engines after a deploy.

Run with: python -m unittest discover -s hooks -p '*_test.py'
"""

import contextlib
import io
import json
import os
import shutil
import sys
import tempfile
import unittest
import unittest.mock
from pathlib import Path

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import indexnow  # noqa: E402

SITEMAP = """<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
{}</urlset>
"""


def sitemap(pages: dict[str, str]) -> str:
    rows = ""
    for loc, date in pages.items():
        rows += f"  <url>\n    <loc>{loc}</loc>\n"
        if date:
            rows += f"    <lastmod>{date}</lastmod>\n"
        rows += "  </url>\n"
    return SITEMAP.format(rows)


class Changed(unittest.TestCase):
    def test_a_page_the_live_site_has_never_listed_is_submitted(self):
        built = {"https://reet.sh/": "2026-01-01", "https://reet.sh/new/": "2026-01-01"}
        live = {"https://reet.sh/": "2026-01-01"}
        self.assertEqual(indexnow.changed(built, live), ["https://reet.sh/new/"])

    def test_a_page_edited_since_the_live_site_was_built_is_submitted(self):
        built = {"https://reet.sh/a/": "2026-02-01", "https://reet.sh/b/": "2026-01-01"}
        live = {"https://reet.sh/a/": "2026-01-01", "https://reet.sh/b/": "2026-01-01"}
        self.assertEqual(indexnow.changed(built, live), ["https://reet.sh/a/"])

    def test_a_second_edit_the_same_day_is_submitted(self):
        built = {"https://reet.sh/a/": "2026-02-01T10:00:00Z"}
        live = {"https://reet.sh/a/": "2026-02-01T09:00:00Z"}
        self.assertEqual(indexnow.changed(built, live), ["https://reet.sh/a/"])

    def test_a_page_with_the_same_date_is_left_alone(self):
        pages = {"https://reet.sh/a/": "2026-01-01"}
        self.assertEqual(indexnow.changed(pages, dict(pages)), [])

    def test_the_first_deploy_with_dates_submits_every_page_once(self):
        # The live sitemap from before dates existed lists the same URLs with no
        # lastmod, so each one differs and the whole site is announced once.
        built = {"https://reet.sh/a/": "2026-01-01", "https://reet.sh/b/": "2026-01-02"}
        live = {"https://reet.sh/a/": "", "https://reet.sh/b/": ""}
        self.assertEqual(sorted(indexnow.changed(built, live)), sorted(built))

    def test_an_undated_build_does_not_resubmit_pages_that_are_already_live(self):
        # A shallow checkout has no history, so nothing is dated. That must read
        # as "cannot tell", not as "everything changed".
        built = {"https://reet.sh/a/": "", "https://reet.sh/fresh/": ""}
        live = {"https://reet.sh/a/": "2026-01-01"}
        self.assertEqual(indexnow.changed(built, live), ["https://reet.sh/fresh/"])


class Payload(unittest.TestCase):
    def test_it_names_the_host_and_looks_for_the_key_beside_the_sitemap(self):
        body = json.loads(indexnow.payload(["https://reet.sh/a/"], "k3y", "https://reet.sh/sitemap.xml"))
        self.assertEqual(
            body,
            {
                "host": "reet.sh",
                "key": "k3y",
                "keyLocation": "https://reet.sh/k3y.txt",
                "urlList": ["https://reet.sh/a/"],
            },
        )

    def test_a_site_served_under_a_path_keeps_its_key_under_that_path(self):
        # IndexNow only believes a key file for URLs at or below its directory.
        body = json.loads(
            indexnow.payload(["https://reet.sh/docs/a/"], "k3y", "https://reet.sh/docs/sitemap.xml")
        )
        self.assertEqual(body["keyLocation"], "https://reet.sh/docs/k3y.txt")

    def test_nothing_to_submit_is_an_empty_payload(self):
        self.assertEqual(indexnow.payload([], "k3y", "https://reet.sh/sitemap.xml"), "")

    def test_one_request_never_carries_more_than_the_protocol_allows(self):
        urls = [f"https://reet.sh/{n}/" for n in range(indexnow.LIMIT + 5)]
        body = json.loads(indexnow.payload(urls, "k3y", "https://reet.sh/sitemap.xml"))
        self.assertEqual(len(body["urlList"]), indexnow.LIMIT)


class Main(unittest.TestCase):
    def setUp(self):
        self.dir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.dir, ignore_errors=True)
        self.output = self.dir / "github_output"
        self.env = unittest.mock.patch.dict(os.environ, {"GITHUB_OUTPUT": str(self.output)})
        self.env.start()
        self.addCleanup(self.env.stop)

    def run_main(self, built: dict[str, str], live: str | None, robots: str | None = "Allow: /") -> str:
        site = self.dir / "site"
        site.mkdir(exist_ok=True)
        (site / "sitemap.xml").write_text(sitemap(built), encoding="utf-8")
        if robots is not None:
            (site / "robots.txt").write_text(f"User-agent: *\n{robots}\n", encoding="utf-8")
        live_path = self.dir / "live.xml"
        if live is not None:
            live_path.write_text(live, encoding="utf-8")
        with contextlib.redirect_stdout(io.StringIO()):
            code = indexnow.main(["indexnow.py", str(site / "sitemap.xml"), live_path.as_uri(), "k3y"])
        self.assertEqual(code, 0)
        written = self.output.read_text(encoding="utf-8")
        self.assertTrue(written.startswith("payload="), written)
        return written[len("payload="):].strip()

    def test_it_submits_what_changed_against_the_live_sitemap(self):
        built = {"https://reet.sh/a/": "2026-02-01", "https://reet.sh/b/": "2026-01-01"}
        live = sitemap({"https://reet.sh/a/": "2026-01-01", "https://reet.sh/b/": "2026-01-01"})
        body = json.loads(self.run_main(built, live))
        self.assertEqual(body["urlList"], ["https://reet.sh/a/"])

    def test_it_submits_nothing_when_the_live_sitemap_cannot_be_read(self):
        # A deploy must never announce the whole site because the comparison
        # could not be made.
        self.assertEqual(self.run_main({"https://reet.sh/a/": "2026-02-01"}, None), "")

    def test_it_submits_nothing_for_a_build_that_asks_crawlers_to_stay_away(self):
        built = {"https://reet.sh/a/": "2026-02-01"}
        live = sitemap({})
        self.assertEqual(self.run_main(built, live, robots="Disallow: /"), "")

    def test_it_submits_nothing_when_nothing_changed(self):
        pages = {"https://reet.sh/a/": "2026-01-01"}
        self.assertEqual(self.run_main(pages, sitemap(pages)), "")


class IndexingAllowed(unittest.TestCase):
    def check(self, robots: str | None) -> bool:
        with tempfile.TemporaryDirectory() as tmp:
            if robots is not None:
                Path(tmp, "robots.txt").write_text(robots, encoding="utf-8")
            return indexnow.indexing_allowed(os.path.join(tmp, "sitemap.xml"))

    def test_a_site_that_allows_crawling_may_be_submitted(self):
        self.assertTrue(self.check("User-agent: *\nAllow: /\nSitemap: https://reet.sh/sitemap.xml\n"))

    def test_a_site_that_disallows_everything_may_not(self):
        self.assertFalse(self.check("User-agent: *\nDisallow: /\n"))

    def test_a_disallowed_subpath_does_not_stop_the_rest_of_the_site(self):
        self.assertTrue(self.check("User-agent: *\nDisallow: /private/\n"))

    def test_a_build_without_a_robots_file_is_not_treated_as_a_preview(self):
        self.assertTrue(self.check(None))


if __name__ == "__main__":
    unittest.main()
