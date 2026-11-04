#!/usr/bin/env python3
"""Smoke-test the real static docs in Chromium; never starts an app or an agent.

Install scripts/requirements-docs-browser.txt and `playwright install chromium`.
The server binds only loopback and serves only the generated public site.
"""
from __future__ import annotations

import argparse
import re
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread

from playwright.sync_api import expect, sync_playwright


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *_args):
        pass


def check(site_dir, executable=None, screenshots=None):
    handler = partial(QuietHandler, directory=str(site_dir.resolve()))
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base = f"http://127.0.0.1:{server.server_port}/"
    if screenshots:
        screenshots.mkdir(parents=True, exist_ok=True)
    try:
        with sync_playwright() as runtime:
            options = {"headless": True}
            if executable:
                options["executable_path"] = executable
            browser = runtime.chromium.launch(**options)
            page = browser.new_page(viewport={"width": 1440, "height": 1000})
            errors = []
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.goto(base)
            page.get_by_role("tab", name="03 Skills & sync").click()
            expect(page.locator("#tour-skills")).to_be_visible()
            page.get_by_role("tab", name="03 Skills & sync").press("ArrowRight")
            expect(page.locator("#tour-gaffer")).to_be_visible()
            # Input events include context-menu paste and touch/speech entry.
            page.get_by_role("button", name="Search documentation", exact=True).click()
            search = page.locator(".md-search__input")
            expect(search).to_be_focused()
            search.fill("worktree")
            result = page.locator(".md-search-result__link").first
            expect(result).to_be_in_viewport(timeout=15000)
            assert page.locator(".md-search__overlay").bounding_box()["height"] >= 990
            result.click()
            expect(page).to_have_url(re.compile(r".*worktree.*"))
            page.keyboard.press("Escape")
            expect(page.locator("#__search")).not_to_be_checked()
            page.get_by_role("button", name="Switch to dark theme", exact=True).click()
            expect(page.locator("body")).to_have_attribute("data-md-color-scheme", "slate")
            page.goto(base + "quick-start/")
            expect(page.locator("body")).to_have_attribute("data-md-color-scheme", "slate")
            page.get_by_role("button", name="Switch to light theme", exact=True).click()
            page.goto(base + "guides/")
            page.get_by_role("button", name="Skills & sync", exact=True).click()
            expect(page.locator("[data-category]:visible")).to_have_count(1)
            page.reload()
            expect(page.locator("[data-category]:visible")).to_have_count(1)
            page.get_by_role("button", name="All guides", exact=False).click()
            expect(page.locator("[data-category]:visible")).to_have_count(8)
            page.goto(base)
            page.set_viewport_size({"width": 390, "height": 844})
            page.get_by_role("button", name="Open navigation", exact=True).click()
            expect(page.locator("#__drawer")).to_be_checked()
            expect(page.locator("#reet-navigation")).to_be_in_viewport()
            drawer_header = page.locator(".reet-drawer-head")
            expect(drawer_header.locator("img:visible")).to_have_count(1)
            assert drawer_header.bounding_box()["height"] < 100
            if screenshots:
                page.screenshot(path=str(screenshots / "mobile-navigation-light.png"))
            mobile_menu = page.locator(".reet-mobile-menu")
            mobile_menu.get_by_text("Documentation", exact=True).click()
            expect(mobile_menu.get_by_role("link", name="Quick start", exact=True)).to_be_visible()
            page.get_by_role("button", name="Close navigation", exact=True).click()
            expect(page.locator("#__drawer")).not_to_be_checked()
            page.get_by_role("button", name="Switch to dark theme", exact=True).click()
            page.get_by_role("button", name="Open navigation", exact=True).click()
            expect(drawer_header.locator("img:visible")).to_have_count(1)
            if screenshots:
                page.screenshot(path=str(screenshots / "mobile-navigation-dark.png"))
            page.get_by_role("button", name="Close navigation", exact=True).click()
            page.get_by_role("button", name="Switch to light theme", exact=True).click()
            page.get_by_role("button", name="Open navigation", exact=True).click()
            page.keyboard.press("Escape")
            expect(page.locator("#__drawer")).not_to_be_checked()
            expect(page.locator("#reet-navigation")).not_to_be_in_viewport()
            # Eight layouts, including a real error page, at phone/tablet/desktop sizes.
            routes = ["", "quick-start/", "documentation/", "guides/", "run-claude-code-on-a-vps/", "run-codex-in-a-browser/", "compare/", "404.html"]
            for width in [360, 390, 768, 1024, 1440]:
                page.set_viewport_size({"width": width, "height": 900})
                for route in routes:
                    page.goto(base + route)
                    assert not page.evaluate("document.documentElement.scrollWidth > innerWidth"), (route, width)
                    expect(page.locator("h1")).to_have_count(1)
                    if width < 1220:
                        expect(page.locator("#reet-navigation")).not_to_be_in_viewport()
                    if screenshots and width in [390, 1440] and route in ["", "quick-start/", "guides/", "run-claude-code-on-a-vps/"]:
                        page.evaluate("document.fonts.ready")
                        page.screenshot(path=str(screenshots / f"{route.strip('/') or 'home'}-{width}.png"), full_page=True)
            assert not errors, errors
            nojs = browser.new_context(java_script_enabled=False, viewport={"width": 390, "height": 844})
            reader = nojs.new_page()
            reader.goto(base)
            expect(reader.locator(".tour-panel:visible")).to_have_count(4)
            reader.get_by_role("button", name="Open navigation", exact=True).click()
            expect(reader.locator("#__drawer")).to_be_checked()
            reader.locator(".reet-mobile-menu").get_by_text("Documentation", exact=True).click()
            expect(reader.locator(".reet-mobile-menu").get_by_role("link", name="Quick start", exact=True)).to_be_visible()
            reader.goto(base + "guides/")
            expect(reader.locator(".article-card:visible")).to_have_count(8)
            browser.close()
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    print("Browser checks passed: search, result navigation, product tabs, themes, filters, mobile drawer, 40 responsive page views, and no-JS reading/navigation.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("site_dir", type=Path, nargs="?", default=Path("site"))
    parser.add_argument("--browser-executable")
    parser.add_argument("--screenshots", type=Path)
    args = parser.parse_args()
    check(args.site_dir, args.browser_executable, args.screenshots)
