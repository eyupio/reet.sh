"""Post-build files the docs site needs but MkDocs does not write by default.

The canonical origin comes from mkdocs.yml (`site_url`) and may include a path,
so the same build works at https://reet.sh/docs/ or at a Pages origin. A page is
either an index.html (the directory URL) or a plain .html file; 404.html is left
out because it is not a public destination.
"""

from __future__ import annotations

import html
import os
import re
from xml.sax.saxutils import escape

TITLE = re.compile(r"<title>(.*?)</title>", re.I | re.S)
DESCRIPTION = re.compile(r'<meta\s+name="description"\s+content="([^"]*)"', re.I | re.S)
PARAGRAPH = re.compile(r"^(?!#|!\[|\[|<|[-*+]\s|\d+\.\s)(.+)$", re.M)
MARKDOWN = re.compile(r"[#*_`>\[\]()]")
SPACES = re.compile(r"\s+")


def on_page_markdown(markdown, page, config, files):
    """Give each page its own description instead of repeating site_description.

    Material reads page.meta.description for the head and sharing tags. The
    docs already start with a short paragraph, so derive the description from it
    and leave front matter alone when a page supplies one.
    """
    if page.meta.get("description"):
        return markdown
    match = PARAGRAPH.search(markdown)
    if not match:
        return markdown
    text = SPACES.sub(" ", MARKDOWN.sub("", match.group(1))).strip()
    if not text:
        return markdown
    if len(text) > 155:
        text = text[:152]
        cut = text.rfind(" ")
        if cut > 80:
            text = text[:cut]
        text += "…"
    page.meta["description"] = text
    return markdown


def on_post_build(config, **kwargs):
    site_dir = config["site_dir"]
    base = (config.get("site_url") or "").rstrip("/") + "/"
    pages = []

    for root, _dirs, files in os.walk(site_dir):
        for name in files:
            if not name.endswith(".html") or name == "404.html":
                continue
            absolute = os.path.join(root, name)
            relative = os.path.relpath(absolute, site_dir).replace(os.sep, "/")
            path = relative[: -len("index.html")] if relative.endswith("index.html") else relative
            with open(absolute, encoding="utf-8") as fh:
                source = fh.read()
            title_match = TITLE.search(source)
            desc_match = DESCRIPTION.search(source)
            pages.append(
                {
                    "url": base + path,
                    "title": html.unescape(title_match.group(1)).strip() if title_match else path,
                    "description": html.unescape(desc_match.group(1)).strip() if desc_match else "",
                }
            )

    pages.sort(key=lambda page: page["url"])
    write_sitemap(site_dir, pages)
    write_robots(site_dir, base)
    write_llms(site_dir, pages)
    _write(site_dir, ".nojekyll", "")


def write_sitemap(site_dir, pages):
    lines = [
        '<?xml version="1.0" encoding="UTF-8"?>',
        '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    ]
    for page in pages:
        lines += ["  <url>", "    <loc>" + escape(page["url"]) + "</loc>", "  </url>"]
    lines.append("</urlset>")
    _write(site_dir, "sitemap.xml", "\n".join(lines) + "\n")


def write_robots(site_dir, base):
    body = "\n".join(
        [
            "# reet.sh documentation",
            "User-agent: *",
            "Allow: /",
            "Sitemap: " + base + "sitemap.xml",
            "",
        ]
    )
    _write(site_dir, "robots.txt", body)


def write_llms(site_dir, pages):
    lines = [
        "# reet.sh documentation",
        "",
        "> Install reet.sh, connect machines, run AI agents, sync skills and setups, and operate a self-hosted fleet.",
        "",
        "## Pages",
        "",
    ]
    for page in pages:
        line = "- [" + page["title"] + "](" + page["url"] + ")"
        if page["description"]:
            line += ": " + page["description"]
        lines.append(line)
    lines.append("")
    _write(site_dir, "llms.txt", "\n".join(lines))


def _write(site_dir, name, body):
    with open(os.path.join(site_dir, name), "w", encoding="utf-8") as fh:
        fh.write(body)
