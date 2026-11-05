"""Which pages to tell search engines about after a deploy, as an IndexNow payload.

This is not an MkDocs hook (mkdocs.yml does not register it) but it lives beside
them because it reads what they write: the sitemap hooks/docs.py renders, where
every URL carries the time its page was last edited in git.

A crawler finds a changed page on its own eventually, through that sitemap.
IndexNow (https://www.indexnow.org) is the way to say so the moment it ships:
one POST, and Bing, Yandex, Seznam, Naver and the rest are told which URLs
changed. Google does not take part; it reads the sitemap.

It submits only what changed. The live sitemap, fetched before the new site
replaces it, is the record of what search engines were last shown; a URL that is
new, or whose last-edited time moved, goes in the payload, and nothing else
does. Resubmitting every page on every rebuild would be noise the protocol asks
senders not to make.

Two cases submit nothing on purpose:

* A page the build could not date. A shallow checkout has no history, so no
  page has a time; treating "no time" as "changed" would resubmit the whole site
  on every deploy. Only a URL the live sitemap has never listed is submitted
  without one.
* A build that tells crawlers to stay away (DOCS_NOINDEX, a preview). Telling
  engines about pages the same build marks as not for indexing contradicts it.

The key is not a secret. The protocol proves ownership by the key file being
served beside the sitemap, docs/<key>.txt published with everything else, so
anyone can read it, and a payload naming it is only believed for this host. The
file is looked for next to the sitemap, so a site served under a path (such as
https://reet.sh/docs/) submits its own pages and nobody else's.

    python hooks/indexnow.py site/sitemap.xml https://reet.sh/sitemap.xml KEY

prints the payload, and writes it to $GITHUB_OUTPUT as `payload` when that is
set. An empty payload means there is nothing to submit. It never fails the
build: a live sitemap that cannot be fetched means nothing is submitted, and
says why. Under GitHub Actions that is a warning annotation, so a green run that
compared nothing is visible on its summary and not only in its log.

The request names itself. The sites sit behind a CDN that answers 403 to Python's
default User-Agent (`Python-urllib/3.x`) and 200 to any other, and the first
deploy with this step compared nothing for that reason alone.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.request
import xml.etree.ElementTree as ET
from urllib.parse import urlparse

NS = {"sm": "http://www.sitemaps.org/schemas/sitemap/0.9"}
# The protocol's own ceiling for one request.
LIMIT = 10_000
USER_AGENT = "sitemap-reader/1.0"


def entries(xml: str) -> dict[str, str]:
    """Every <loc> in a sitemap, mapped to its <lastmod> (empty if it has none)."""
    root = ET.fromstring(xml)
    out = {}
    for url in root.findall("sm:url", NS):
        loc = (url.findtext("sm:loc", default="", namespaces=NS) or "").strip()
        if loc:
            out[loc] = (url.findtext("sm:lastmod", default="", namespaces=NS) or "").strip()
    return out


def changed(built: dict[str, str], live: dict[str, str]) -> list[str]:
    """The URLs that are new, or whose last-edited time differs from the live site's."""
    return sorted(
        loc
        for loc, date in built.items()
        # An undated page that is already live is "cannot tell", not "changed".
        if loc not in live or (date and live[loc] != date)
    )


def indexing_allowed(sitemap_path: str) -> bool:
    """False when the build's own robots.txt asks crawlers to stay away."""
    robots = os.path.join(os.path.dirname(sitemap_path) or ".", "robots.txt")
    try:
        with open(robots, encoding="utf-8") as f:
            lines = [line.strip().lower() for line in f]
    except OSError:
        return True
    return "disallow: /" not in lines


def payload(urls: list[str], key: str, sitemap_url: str) -> str:
    """The IndexNow request body for these URLs, or "" when there are none.

    The key file is expected beside the sitemap, whatever path the site is
    served under.
    """
    if not urls:
        return ""
    host = urlparse(urls[0]).netloc
    directory = sitemap_url.rsplit("/", 1)[0]
    return json.dumps(
        {
            "host": host,
            "key": key,
            "keyLocation": f"{directory}/{key}.txt",
            "urlList": urls[:LIMIT],
        },
        separators=(",", ":"),
    )


def main(argv: list[str]) -> int:
    if len(argv) != 4:
        print(__doc__, file=sys.stderr)
        return 2
    built_path, live_url, key = argv[1:]
    with open(built_path, encoding="utf-8") as f:
        built = entries(f.read())
    urls: list[str] = []
    if not indexing_allowed(built_path):
        print("indexnow: this build asks crawlers not to index it; submitting nothing")
    else:
        try:
            request = urllib.request.Request(live_url, headers={"User-Agent": USER_AGENT})
            with urllib.request.urlopen(request, timeout=30) as resp:
                live = entries(resp.read().decode("utf-8"))
        except Exception as err:  # noqa: BLE001 -- any failure means "submit nothing"
            note = f"indexnow: could not read the live sitemap ({err}); submitting nothing"
            print(f"::warning::{note}" if os.environ.get("GITHUB_ACTIONS") else note)
        else:
            urls = changed(built, live)
    if len(urls) > LIMIT:
        print(f"indexnow: {len(urls)} changed URLs; the protocol takes {LIMIT} at a time, so the rest are left to the sitemap")
    body = payload(urls, key, live_url)
    print(f"indexnow: {min(len(urls), LIMIT)} changed URL(s)")
    for url in urls[:LIMIT]:
        print(f"  {url}")
    out = os.environ.get("GITHUB_OUTPUT")
    if out:
        with open(out, "a", encoding="utf-8") as f:
            f.write(f"payload={body}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
