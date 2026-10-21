#!/usr/bin/env python3
"""Builds brand/contact-sheet.html: every variant on light and dark panels."""
from pathlib import Path

OUT = Path(__file__).resolve().parent
rows = []
for d in sorted(p for p in OUT.iterdir() if p.is_dir()):
    cells = []
    for f in sorted(d.glob("*.svg")):
        panel = "dark" if ("color-dark" in f.name or "mono-light" in f.name) else "light"
        if f.name.startswith("tile"):
            panel = "light"
        cells.append(f'<figure class="{panel}"><img src="{d.name}/{f.name}" alt="{f.name}">'
                     f'<figcaption>{f.name}</figcaption></figure>')
    rows.append(f"<h2>{d.name}</h2><div class='grid'>{''.join(cells)}</div>")
html = f"""<!doctype html><meta charset="utf-8"><title>reet.sh logo contact sheet</title>
<style>
body{{margin:0;padding:24px;background:#e9e7e1;font:13px/1.4 system-ui,sans-serif;color:#1d1c1a}}
h2{{margin:28px 0 10px;font-size:15px}}
.grid{{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}}
figure{{margin:0;padding:18px;border-radius:10px;display:flex;flex-direction:column;align-items:center;gap:10px;min-height:120px;justify-content:center}}
figure.light{{background:#f7f6f2}} figure.dark{{background:#151413;color:#a09b90}}
img{{max-width:100%;max-height:110px}}
figcaption{{font-size:11px;opacity:.75}}
</style>
<h1 style="font-size:18px;margin:0">reet.sh logo concepts</h1>
{''.join(rows)}
"""
(OUT / "contact-sheet.html").write_text(html)
print("wrote contact-sheet.html")
