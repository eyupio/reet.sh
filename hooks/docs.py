"""Post-build files the docs site needs but MkDocs does not write by default.

The canonical origin comes from mkdocs.yml (`site_url`) and may include a path,
so the same build works at https://reet.sh/docs/ or at a Pages origin. A page is
either an index.html (the directory URL) or a plain .html file; 404.html is left
out because it is not a public destination.
"""

from __future__ import annotations

import html
import gzip
import math
import os
import re
from xml.sax.saxutils import escape

from markdown.extensions.toc import slugify

TITLE = re.compile(r"<title>(.*?)</title>", re.I | re.S)
DESCRIPTION = re.compile(r'<meta\s+name="description"\s+content="([^"]*)"', re.I | re.S)
PARAGRAPH = re.compile(r"^(?!#|!\[|\[|<|[-*+]\s|\d+\.\s)(.+)$", re.M)
MARKDOWN = re.compile(r"[#*_`>\[\]()]")
SPACES = re.compile(r"\s+")


def on_page_markdown(markdown, page, config, files):
    """Enrich real Markdown pages; articles and search share the same content."""
    source = page.file.src_uri
    guides = config["extra"].get("field_guides", [])
    page.meta["section"] = (
        "guides" if source in guides or source.startswith("guides/")
        else "compare" if source.startswith(("compare/", "compare-"))
        else "docs"
    )
    if not page.meta.get("description"):
        match = PARAGRAPH.search(markdown)
        if match:
            text = SPACES.sub(" ", MARKDOWN.sub("", match.group(1))).strip()
            if len(text) > 155:
                text = text[:152].rsplit(" ", 1)[0] + "…"
            page.meta["description"] = text or config["site_description"]
    page.meta["read_time"] = max(1, math.ceil(len(re.findall(r"\b[\w’'-]+\b", markdown)) / 220))
    if page.meta.get("layout") == "article":
        heading = re.search(r"^# (.+)$", markdown, re.M)
        title = heading.group(1).strip() if heading else page.title
        page.meta["heading_id"] = slugify(title, "-")
        page.meta.setdefault("hero_title", title)
        if heading:
            # The article template renders this heading once, keeping its anchor.
            markdown = markdown[:heading.start()] + markdown[heading.end():]
    return markdown


def on_page_context(context, page, config, nav, **kwargs):
    by_source = {item.file.src_uri: item for item in nav.pages}

    def pages_for(paths):
        return [by_source[path] for path in paths if path in by_source]

    context["guide_pages"] = pages_for(config["extra"].get("field_guides", []))
    context["compare_pages"] = pages_for([
        "compare-skills-sh.md", "compare-open-webui.md", "compare-librechat.md",
    ])
    context["related_pages"] = pages_for(page.meta.get("related", []))
    base = config["site_url"]
    section = page.meta.get("section", "docs")
    hub, label = {
        "guides": ("guides/", "Field guide"),
        "compare": ("compare/", "Compare"),
        "docs": ("documentation/", "Documentation"),
    }[section]
    crumbs = [{"@type": "ListItem", "position": 1, "name": "Home", "item": base}]
    if not page.is_homepage:
        if page.canonical_url != base + hub:
            crumbs.append({"@type": "ListItem", "position": 2, "name": label, "item": base + hub})
        crumbs.append({"@type": "ListItem", "position": len(crumbs) + 1, "name": page.title, "item": page.canonical_url})
    graph = [{
        "@type": "WebPage" if page.is_homepage or page.meta.get("layout") in ["hub", "guides", "compare"] else "TechArticle",
        "@id": page.canonical_url,
        "url": page.canonical_url,
        "name": page.title,
        "description": page.meta.get("description", config["site_description"]),
        "inLanguage": "en",
        "isPartOf": {"@id": base + "#website"},
        "publisher": {"@type": "Organization", "name": "EyUp.io", "url": "https://eyup.io"},
    }, {"@type": "BreadcrumbList", "itemListElement": crumbs}]
    if graph[0]["@type"] == "TechArticle":
        graph[0]["headline"] = " ".join(filter(None, [page.meta.get("hero_title", page.title), page.meta.get("hero_accent")]))
        graph[0]["image"] = base + "assets/brand/social.png"
        graph[0]["author"] = {"@type": "Organization", "name": config["site_author"], "url": "https://eyup.io"}
    if page.is_homepage:
        graph += [{
            "@type": "WebSite", "@id": base + "#website", "url": base,
            "name": "reet.sh", "description": config["site_description"],
        }, {
            "@type": "SoftwareApplication", "name": "reet.sh", "url": base,
            "applicationCategory": "DeveloperApplication", "operatingSystem": "Linux, macOS",
            "license": config["repo_url"] + "/blob/main/LICENSE",
            "isAccessibleForFree": True, "sameAs": config["repo_url"],
        }]
    page.meta["schema"] = {"@context": "https://schema.org", "@graph": graph}
    return context


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
    write_robots(site_dir, base, config["extra"].get("noindex", False))
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
    body = "\n".join(lines) + "\n"
    _write(site_dir, "sitemap.xml", body)
    # MkDocs also emits a compressed sitemap. Keep the two representations equal.
    with open(os.path.join(site_dir, "sitemap.xml.gz"), "wb") as fh:
        fh.write(gzip.compress(body.encode("utf-8"), mtime=0))


def write_robots(site_dir, base, noindex=False):
    body = "\n".join(
        [
            "# reet.sh documentation",
            "User-agent: *",
            "Disallow: /" if noindex else "Allow: /",
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
