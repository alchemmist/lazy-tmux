#!/usr/bin/env python3
"""Generate the multi-page lazy-tmux docs site from the section content.

The site is plain static HTML served by GitHub Pages (no runtime build). This
script is the single source of truth for the shared <head>/sidebar/footer
template and the page split; run it to regenerate the static files:

    python3 scripts/build-docs.py

It reads the section bodies from docs/_content.html (a flat document with one
<details class="section"> per section, plus the hero and footer), extracts the
shared CSS into assets/style.css, and writes index.html plus one
<page>/index.html per documentation page.
"""

import os
import re

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CONTENT = os.path.join(ROOT, "docs", "_content.html")

GITHUB = "https://github.com/alchemmist/lazy-tmux"
SITE = "https://lazy-tmux.xyz"

# Sidebar navigation: (label, href, slug). slug "" marks the home page.
NAV = [
    ("Home", "/", "home"),
    ("Features", "/features/", "features"),
    ("Installation", "/installation/", "installation"),
    ("tmux setup", "/tmux-setup/", "tmux-setup"),
    ("CLI", "/cli/", "cli"),
    ("Configuration", "/configuration/", "configuration"),
    ("TUI picker", "/tui-picker/", "tui-picker"),
    ("About", "/about/", "about"),
]

# Pages: slug -> (output path, <title>, meta description, [section titles]).
# "@hero" and "@footer" are special tokens handled in render().
PAGES = {
    "home": (
        "index.html",
        "lazy-tmux: lazy tmux session manager with scrollback restore",
        "Forget bugs of tmux-resurrect and tmux-continuum. lazy-tmux restores only the sessions you need, preserves scrollback history, and works instantly.",
        ["@hero", "Demo Preview"],
    ),
    "features": (
        "features/index.html",
        "Features — lazy-tmux",
        "What lazy-tmux can do: lazy restore, scrollback capture, TUI picker, autosave daemon, config file and restore allowlist.",
        ["Features"],
    ),
    "installation": (
        "installation/index.html",
        "Installation — lazy-tmux",
        "Install lazy-tmux via the install script, Arch (AUR) or Homebrew.",
        ["Installation"],
    ),
    "tmux-setup": (
        "tmux-setup/index.html",
        "Quick tmux setup — lazy-tmux",
        "Wire lazy-tmux into your tmux config: picker keybind, autosave daemon and save shortcut.",
        ["Quick tmux setup"],
    ),
    "cli": (
        "cli/index.html",
        "CLI reference — lazy-tmux",
        "lazy-tmux commands and flags: save, restore, picker, daemon, sorting, scrollback and storage.",
        ["CLI"],
    ),
    "configuration": (
        "configuration/index.html",
        "Configuration — lazy-tmux",
        "lazy-tmux TOML config file: lookup order, precedence, restore command allowlist and restore settle timeout.",
        ["Configuration"],
    ),
    "tui-picker": (
        "tui-picker/index.html",
        "TUI picker — lazy-tmux",
        "Keyboard-driven lazy-tmux TUI picker: navigation and management keybinds.",
        ["TUI picker"],
    ),
    "about": (
        "about/index.html",
        "About — lazy-tmux",
        "Why lazy-tmux exists and the philosophy behind it.",
        ["Why?", "Philosophy"],
    ),
}

LD_JSON = """<script type="application/ld+json">
      {
        "@context": "https://schema.org",
        "@type": "SoftwareApplication",
        "name": "lazy-tmux",
        "description": "Lazy tmux session manager with scrollback restore, alternative to tmux-resurrect and tmux-continuum without bugs",
        "applicationCategory": "Utility",
        "operatingSystem": "Linux, macOS, BSD",
        "url": "https://lazy-tmux.xyz",
        "sameAs": "https://github.com/alchemmist/lazy-tmux",
        "downloadUrl": "https://github.com/alchemmist/lazy-tmux/releases",
        "author": {
          "@type": "Person",
          "name": "Anton Grishin (alchemmist)",
          "email": "anton.ingrish@gmail.com"
        }
      }
    </script>"""

LAYOUT_CSS = """
/* ---- multi-page layout ---- */
.nav-toggle-cb { position: absolute; opacity: 0; pointer-events: none; }
.sidebar {
  position: fixed;
  top: 0;
  left: 0;
  width: 248px;
  height: 100vh;
  border-right: 1px solid var(--line);
  background: rgba(8, 8, 8, 0.7);
  padding: 26px 16px;
  overflow-y: auto;
  z-index: 5;
}
.sidebar .brand {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  color: var(--text);
  margin-bottom: 24px;
}
.sidebar .brand img { width: 34px; height: auto; }
.sidebar .brand span { font-family: "Pixelify Sans", monospace; font-size: 24px; }
.sidebar nav { display: flex; flex-direction: column; gap: 2px; }
.sidebar nav a {
  color: var(--muted);
  text-decoration: none;
  padding: 8px 10px;
  border-left: 2px solid transparent;
  font-size: 15px;
}
.sidebar nav a:hover { color: var(--text); background: rgba(255, 255, 255, 0.03); }
.sidebar nav a.active {
  color: var(--text);
  border-left-color: var(--accent);
  background: rgba(255, 255, 255, 0.05);
}
.sidebar nav a.external { margin-top: 16px; color: rgb(228, 242, 247); }
.content { margin-left: 248px; padding: 44px 0 64px; position: relative; z-index: 1; }
.page { width: min(900px, 90vw); margin: 0 auto; padding: 0 16px; }
.doc-section { margin-bottom: 30px; }
.doc-section > h1 {
  font-family: "Pixelify Sans", monospace;
  font-size: 34px;
  letter-spacing: 0.4px;
  margin: 0 0 18px;
}
.topbar { display: none; }
/* home hero, centered */
.home .hero { text-align: center; border: none; background: none; padding: 8px 0 0; }
.home .hero > div { justify-content: center; }
.home .actions { justify-content: center; }
.home .doc-section { text-align: left; }
@media (max-width: 860px) {
  .sidebar {
    transform: translateX(-100%);
    transition: transform 0.2s ease;
  }
  .nav-toggle-cb:checked ~ .sidebar { transform: translateX(0); }
  .content { margin-left: 0; padding-top: 66px; }
  .topbar {
    display: flex;
    align-items: center;
    gap: 12px;
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    height: 52px;
    padding: 0 14px;
    background: var(--bg);
    border-bottom: 1px solid var(--line);
    z-index: 6;
  }
  .topbar label.nav-toggle { cursor: pointer; font-size: 22px; line-height: 1; }
  .topbar .topbar-brand {
    color: var(--text);
    text-decoration: none;
    font-family: "Pixelify Sans", monospace;
    font-size: 20px;
  }
}
"""

# Old single-page hash anchors -> new page URLs, so existing inbound links work.
HASH_REDIRECT = """<script>
      (function () {
        var map = {
          "#features": "/features/",
          "#install": "/installation/",
          "#cli": "/cli/",
          "#config": "/configuration/"
        };
        var dest = map[window.location.hash];
        if (dest) window.location.replace(dest);
      })();
    </script>"""


def extract(pattern, text, flags=re.S):
    m = re.search(pattern, text, flags)
    if not m:
        raise SystemExit("pattern not found: " + pattern)
    return m.group(1)


def main():
    html = open(CONTENT, encoding="utf-8").read()

    style = extract(r"<style>(.*?)</style>", html)
    hero = extract(r'<section class="hero">(.*?)</section>', html)
    footer = "<footer>" + extract(r"<footer>(.*?)</footer>", html) + "</footer>"

    # Point the hero CTA buttons at their new pages.
    hero = hero.replace('href="#install"', 'href="/installation/"')
    hero = hero.replace('href="#cli"', 'href="/cli/"')
    hero = '<section class="hero">' + hero + "</section>"

    # Collect sections by their <h2> title.
    sections = {}
    for m in re.finditer(
        r'<details class="section"[^>]*>\s*<summary><h2>(.*?)</h2></summary>\s*'
        r'<div class="detail-body">(.*?)</div>\s*</details>',
        html,
        re.S,
    ):
        sections[m.group(1).strip()] = m.group(2).strip()

    # Write the shared stylesheet.
    os.makedirs(os.path.join(ROOT, "assets"), exist_ok=True)
    with open(os.path.join(ROOT, "assets", "style.css"), "w", encoding="utf-8") as f:
        f.write(style.strip() + "\n" + LAYOUT_CSS)

    for slug, (path, title, desc, parts) in PAGES.items():
        render(slug, path, title, desc, parts, sections, hero, footer)
        print("wrote", path)


def nav_html(active):
    items = []
    for label, href, slug in NAV:
        cls = ' class="active"' if slug == active else ""
        items.append('<a href="%s"%s>%s</a>' % (href, cls, label))
    items.append('<a class="external" href="%s">GitHub ↗</a>' % GITHUB)
    return "\n        ".join(items)


def render(slug, path, title, desc, parts, sections, hero, footer):
    canonical = SITE + "/" + (path[: -len("index.html")] if path.endswith("index.html") else path)
    canonical = canonical.rstrip("/") + "/" if path != "index.html" else SITE + "/"

    body_parts = []
    for part in parts:
        if part == "@hero":
            body_parts.append(hero)
        else:
            body = sections[part]
            body_parts.append(
                '<section class="doc-section">\n<h1>%s</h1>\n%s\n</section>' % (part, body)
            )
    content = "\n\n      ".join(body_parts)

    ldjson = LD_JSON if slug == "home" else ""
    script = HASH_REDIRECT if slug == "home" else ""
    body_class = "home" if slug == "home" else slug

    out = TEMPLATE.format(
        title=title,
        desc=desc,
        canonical=canonical,
        ldjson=ldjson,
        body_class=body_class,
        nav=nav_html(slug),
        content=content,
        footer=footer,
        script=script,
    )

    full = os.path.join(ROOT, path)
    os.makedirs(os.path.dirname(full) or ".", exist_ok=True)
    with open(full, "w", encoding="utf-8") as f:
        f.write(out)


TEMPLATE = """<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{title}</title>
    <meta name="description" content="{desc}" />
    <link rel="canonical" href="{canonical}" />
    <meta property="og:title" content="{title}" />
    <meta property="og:description" content="{desc}" />
    <meta property="og:type" content="website" />
    <meta property="og:url" content="{canonical}" />
    <meta property="og:image" content="https://lazy-tmux.xyz/assets/banner.png" />
    <meta property="og:image:width" content="1200" />
    <meta property="og:image:height" content="630" />
    <meta name="twitter:card" content="summary_large_image" />
    <link rel="icon" type="image/svg+xml" href="/assets/logo.svg" />
    <link rel="icon" type="image/png" href="/assets/favicon-96x96.png" sizes="96x96" />
    <link rel="icon" type="image/svg+xml" href="/assets/favicon.svg" />
    <link rel="shortcut icon" href="/assets/favicon.ico" />
    <link rel="apple-touch-icon" sizes="180x180" href="/assets/apple-touch-icon.png" />
    <link rel="manifest" href="/assets/site.webmanifest" />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=Pixelify+Sans:wght@400;600;700&display=swap" rel="stylesheet" />
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&display=swap" rel="stylesheet" />
    <link rel="stylesheet" href="/assets/style.css" />
    <script defer src="https://umami.alchemmist.xyz/script.js" data-website-id="1d4c3792-8068-4e17-a321-f5ecd6eeea4e"></script>
    {ldjson}
  </head>
  <body class="{body_class}">
    <div class="grid" aria-hidden="true"></div>
    <input type="checkbox" id="nav-toggle" class="nav-toggle-cb" />
    <header class="topbar">
      <label for="nav-toggle" class="nav-toggle" aria-label="Toggle menu">☰</label>
      <a href="/" class="topbar-brand">lazy-tmux</a>
    </header>
    <aside class="sidebar">
      <a class="brand" href="/">
        <img src="/assets/logo-white.svg" alt="lazy-tmux logo" />
        <span>lazy-tmux</span>
      </a>
      <nav>
        {nav}
      </nav>
    </aside>
    <main class="content">
      <div class="page">
        {content}
        {footer}
      </div>
    </main>
    {script}
  </body>
</html>
"""


if __name__ == "__main__":
    main()
