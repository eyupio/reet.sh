#!/usr/bin/env python3
"""Check the rendered docs, including links generated outside Markdown.

Run after `mkdocs build --strict`. Uses only the Python standard library.
"""
from __future__ import annotations

import argparse
import gzip
import json
import re
import struct
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urldefrag, urljoin, urlparse
from xml.etree import ElementTree


class Document(HTMLParser):
    def __init__(self, source):
        super().__init__(convert_charrefs=True)
        self.links = []
        self.ids = set()
        self.canonicals = []
        self.descriptions = []
        self.og_images = []
        self.schemas = []
        self.meta = {}
        self.title_count = 0
        self.title = ""
        self.h1_count = 0
        self._in_title = False
        self._schema = None
        self.feed(source)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "meta":
            key = attrs.get("property") or attrs.get("name")
            if key:
                self.meta.setdefault(key, []).append(attrs.get("content", ""))
        if attrs.get("id"):
            self.ids.add(attrs["id"])
        if tag == "h1":
            self.h1_count += 1
        if tag == "title":
            self.title_count += 1
            self._in_title = True
        if tag == "link" and attrs.get("rel") == "canonical":
            self.canonicals.append(attrs.get("href", ""))
        if tag == "meta" and attrs.get("name") == "description":
            self.descriptions.append(attrs.get("content", ""))
        if tag == "meta" and attrs.get("property") == "og:image":
            self.og_images.append(attrs.get("content", ""))
        if tag == "script" and attrs.get("type") == "application/ld+json":
            self._schema = ""
        for name in ("href", "src"):
            if attrs.get(name):
                self.links.append(attrs[name])
        if attrs.get("srcset"):
            self.links.extend(part.strip().split()[0] for part in attrs["srcset"].split(","))

    def handle_data(self, data):
        if self._in_title:
            self.title += data
        if self._schema is not None:
            self._schema += data

    def handle_endtag(self, tag):
        if tag == "title":
            self._in_title = False
        if tag == "script" and self._schema is not None:
            self.schemas.append(json.loads(self._schema))
            self._schema = None


def check(site_dir):
    site_dir = site_dir.resolve()
    documents = {}
    errors = []
    for path in site_dir.rglob("*.html"):
        documents[path] = Document(path.read_text(encoding="utf-8"))
    home = documents.get(site_dir / "index.html")
    if not home or len(home.canonicals) != 1:
        raise SystemExit("Home page must have exactly one canonical URL")
    base = home.canonicals[0]
    origin = urlparse(base)
    public_urls = set()
    titles = set()
    descriptions = set()
    preview = "noindex" in " ".join(home.meta.get("robots", []))

    def error(path, message):
        errors.append(f"{path.relative_to(site_dir)}: {message}")

    def destination(url):
        parsed = urlparse(url)
        if parsed.scheme not in ("http", "https") or parsed.netloc != origin.netloc:
            return None
        if not parsed.path.startswith(origin.path):
            return None
        relative = unquote(parsed.path[len(origin.path):])
        path = site_dir / relative
        if parsed.path.endswith("/") or path.is_dir():
            path /= "index.html"
        return path

    for path, document in documents.items():
        relative = path.relative_to(site_dir).as_posix()
        page_url = base + (relative[:-10] if relative.endswith("index.html") else relative)
        if path.name != "404.html":
            public_urls.add(page_url)
            if document.canonicals != [page_url]:
                error(path, f"expected one canonical for {page_url}, got {document.canonicals}")
            if document.title_count != 1 or not document.title.strip() or document.title in titles:
                error(path, "empty or duplicate title")
            titles.add(document.title)
            if len(document.descriptions) != 1 or not document.descriptions[0].strip():
                error(path, "expected one nonempty description")
            elif document.descriptions[0] in descriptions:
                error(path, "duplicate description")
            descriptions.update(document.descriptions)
            required_meta = [
                "robots", "og:type", "og:site_name", "og:title", "og:description", "og:url",
                "og:image", "og:image:width", "og:image:height", "og:image:type", "og:image:alt",
                "twitter:card", "twitter:title", "twitter:description", "twitter:image", "twitter:image:alt",
            ]
            if origin.scheme == "https":
                required_meta.append("og:image:secure_url")
            for key in required_meta:
                values = document.meta.get(key, [])
                if len(values) != 1 or not values[0].strip():
                    error(path, f"expected exactly one nonempty {key} tag")
            expected_values = {
                "og:url": page_url, "og:title": document.title, "twitter:title": document.title,
                "og:description": document.descriptions[0] if document.descriptions else "",
                "twitter:description": document.descriptions[0] if document.descriptions else "",
                "twitter:card": "summary_large_image", "og:image:type": "image/png",
                "robots": "noindex,nofollow" if preview else "index,follow,max-image-preview:large",
            }
            image = document.og_images[0] if len(document.og_images) == 1 else ""
            expected_values["twitter:image"] = image
            expected_values["twitter:image:alt"] = document.meta.get("og:image:alt", [""])[0]
            if origin.scheme == "https":
                expected_values["og:image:secure_url"] = image
            for key, expected in expected_values.items():
                if document.meta.get(key) != [expected]:
                    error(path, f"{key} disagrees with this page's title, description, canonical or sharing image")
            if document.h1_count != 1:
                error(path, f"expected one h1, got {document.h1_count}")
            if len(document.og_images) != 1:
                error(path, "expected one social sharing image")
            for image in document.og_images:
                target = destination(image)
                if not target or not target.is_file():
                    error(path, f"missing social image: {image}")
                else:
                    data = target.read_bytes()
                    if not data.startswith(b"\x89PNG\r\n\x1a\n"):
                        error(path, "social image is not a PNG")
                    else:
                        width, height = struct.unpack(">II", data[16:24])
                        declared = (document.meta.get("og:image:width"), document.meta.get("og:image:height"))
                        if declared != ([str(width)], [str(height)]) or width < 1200 or height < 630:
                            error(path, "social image dimensions differ from the tags or are too small for a large card")
            if len(document.schemas) != 1:
                error(path, "expected one structured-data graph")
            else:
                graph = document.schemas[0].get("@graph", [])
                crumbs = next((item for item in graph if item.get("@type") == "BreadcrumbList"), {})
                entries = crumbs.get("itemListElement", [])
                if not entries or entries[-1].get("item") != page_url:
                    error(path, "breadcrumb does not end at this page")
        elif document.meta.get("robots") != ["noindex,nofollow"]:
            error(path, "404 page must have exactly one noindex robots tag")
        for link in document.links:
            url, fragment = urldefrag(urljoin(page_url, link))
            target = destination(url)
            if target is None:
                continue
            if not target.is_file():
                error(path, f"missing local destination: {link}")
            elif fragment and target in documents and unquote(fragment) not in documents[target].ids:
                error(path, f"missing anchor: {link}")

    for path in (site_dir / "assets").rglob("*.css"):
        for link in re.findall(r"url\(\s*['\"]?([^)'\"\s]+)", path.read_text(encoding="utf-8")):
            if link.startswith(("data:", "http:", "https:", "#")):
                continue
            if not (path.parent / unquote(link.split("?", 1)[0])).is_file():
                error(path, f"missing CSS asset: {link}")

    sitemap = (site_dir / "sitemap.xml").read_bytes()
    sitemap_tree = ElementTree.fromstring(sitemap)
    sitemap_namespace = "{http://www.sitemaps.org/schemas/sitemap/0.9}"
    locations = {node.text for node in sitemap_tree.iter(sitemap_namespace + "loc")}
    if locations != public_urls:
        errors.append(f"sitemap differs from rendered pages: {locations ^ public_urls}")
    undated = [
        node.text
        for node in sitemap_tree.iter(sitemap_namespace + "lastmod")
        if not re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", node.text or "")
    ]
    if undated:
        errors.append(f"sitemap last-edited times must be UTC, YYYY-MM-DDTHH:MM:SSZ: {undated}")
    if gzip.decompress((site_dir / "sitemap.xml.gz").read_bytes()) != sitemap:
        errors.append("compressed sitemap does not match sitemap.xml")
    if base + "sitemap.xml" not in (site_dir / "robots.txt").read_text():
        errors.append("robots.txt does not link to this deployment's sitemap")
    llms = (site_dir / "llms.txt").read_text()
    if any(url not in llms for url in public_urls):
        errors.append("llms.txt is missing public pages")
    search = json.loads((site_dir / "search/search_index.json").read_text())
    search_locations = {item["location"].split("#", 1)[0] for item in search["docs"]}
    for url in public_urls:
        if url[len(base):] not in search_locations:
            errors.append(f"not in full-text search: {url}")
    if any(any(part in url for part in ("/design/", "/research/", "/handoffs/", "/superpowers/", "/roadmap/")) for url in public_urls):
        errors.append("private design or implementation notes have entered the public site")
    if errors:
        raise SystemExit("\n".join(errors))
    print(f"Docs checks passed: {len(public_urls)} pages; local links, anchors, assets, metadata, schema, search, sitemap and crawler files.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("site_dir", type=Path, nargs="?", default=Path("site"))
    check(parser.parse_args().site_dir)
