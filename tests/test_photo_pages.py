"""Per-photo pages (/p/<id>/) built by Hugo from data/photos.json."""
import json
import re
import shutil
import subprocess
import tomllib
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parent.parent
EXAMPLE = REPO / "exampleSite"
THUMB = "gallery/alpine-dawn-thumb.webp"
PREVIEW = "gallery/alpine-dawn-preview.webp"
_HUGO = tomllib.loads((EXAMPLE / "hugo.toml").read_text())
BASE = _HUGO["baseURL"].rstrip("/")
TITLE = _HUGO["title"]


def _photo(pid, **extra):
    rec = {
        "id": pid, "thumb": THUMB, "preview": PREVIEW,
        "original": f"https://cdn.example.com/originals/{pid}.jpg",
        "width": 3000, "height": 2000, "pw": 1600, "ph": 1067,
        "caption": "", "tags": [], "albums": [], "featured": False,
    }
    rec.update(extra)
    return rec


PHOTOS = [
    _photo("t-a", caption='Say "hi" <now>', albums=["mountains"],
           taken="2026-10-21T11:28:21",
           exif={"camera": "SONY ILCE-6700", "lens": "FE 400mm F4.5 GM OSS",
                 "aperture": "f/6.3", "shutter": "1/320", "iso": 100,
                 "focal": "400mm"}),
    _photo("t-b", taken="2026-10-25T09:19:05"),
    _photo("t-c"),
]


@pytest.fixture(scope="module")
def public(tmp_path_factory):
    if shutil.which("hugo") is None:
        pytest.skip("hugo not installed")
    tmp = tmp_path_factory.mktemp("site")
    site = tmp / "site"
    # The theme is the repo root; Hugo finds it as <themesDir>/contact-sheet.
    themes = tmp / "themes"
    themes.mkdir()
    (themes / "contact-sheet").symlink_to(REPO, target_is_directory=True)
    # exampleSite with its photo catalog replaced by the fixtures above.
    shutil.copytree(EXAMPLE, site,
                    ignore=shutil.ignore_patterns("public", "resources", ".hugo_build.lock"))
    (site / "data" / "photos.json").write_text(json.dumps(PHOTOS))
    cfg = site / "hugo.toml"
    cfg.write_text(re.sub(r"(?m)^themesDir\s*=.*$", f"themesDir = {json.dumps(str(themes))}",
                          cfg.read_text()))
    out = tmp / "public"
    res = subprocess.run(["hugo", "--destination", str(out)], cwd=site,
                         capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    (tmp / "build.log").write_text(res.stdout + res.stderr)
    return out


def _page(public, pid):
    p = public / "p" / pid / "index.html"
    assert p.exists(), f"missing {p}"
    return p.read_text()


def test_photo_page_has_open_graph(public):
    html = _page(public, "t-a")
    assert 'name="twitter:card" content="summary_large_image"' in html
    assert f'property="og:url" content="{BASE}/p/t-a/"' in html
    assert "&#34;hi&#34;" in html or "&quot;hi&quot;" in html
    assert "<now>" not in html
    assert 'href="/albums/mountains/"' in html


def test_photo_page_without_caption_exif_albums(public):
    html = _page(public, "t-b")
    assert f"<title>25 October 2026 · {TITLE}</title>" in html
    assert "photo-albums" not in html
    exif = re.search(r'class="photo-exif"[^>]*>(.*?)</p>', html, re.S)
    assert exif is None or " · " not in exif.group(1)


def test_photo_page_without_date_uses_id(public):
    html = _page(public, "t-c")
    assert re.search(r"<title>[^<]*t-c[^<]*</title>", html)


def test_photo_pages_not_in_rss(public):
    assert "/p/t-a/" not in (public / "index.xml").read_text()


def test_grid_links_to_photo_page(public):
    html = (public / "albums" / "mountains" / "index.html").read_text()
    assert f'data-page="{BASE}/p/t-a/"' in html


def test_no_photo_section_list_or_feed(public):
    assert not (public / "p" / "index.html").exists()
    assert not (public / "p" / "index.xml").exists()
    assert "/p/</loc>" not in (public / "sitemap.xml").read_text()


def test_og_image_is_1600px_jpg(public):
    html = _page(public, "t-a")
    m = re.search(r'property="og:image" content="' + re.escape(BASE) + r'/([^"]+\.jpg)"', html)
    assert m, "og:image must be a site-hosted JPG"
    assert (public / m.group(1)).is_file()
    assert 'property="og:image:width" content="1600"' in html
    assert re.search(r'name="twitter:image" content="' + re.escape(BASE) + r'/[^"]+\.jpg"', html)
    assert 'href="https://cdn.example.com/originals/t-a.jpg"' in html  # Full size link kept


def test_stable_image_url(public):
    from PIL import Image
    jpg = public / "p" / "t-a" / "t-a.jpg"
    assert jpg.is_file()
    with Image.open(jpg) as im:
        assert im.format == "JPEG" and im.width == 1600
    url = f"{BASE}/p/t-a/t-a.jpg"
    html = _page(public, "t-a")
    assert f'property="og:image" content="{url}"' in html
    assert f'href="{url}"' in html  # "Image link" on the page
    grid = (public / "albums" / "mountains" / "index.html").read_text()
    assert f'data-image="{url}"' in grid


def test_build_has_no_deprecation_warnings(public):
    # Hugo deprecates config keys and template functions a few releases before
    # removing them; a warning today is a broken build later.
    log = (public.parent / "build.log").read_text()
    assert "deprecated" not in log.lower(), log


def test_html_lang_comes_from_locale(public):
    html = (public / "index.html").read_text()
    assert '<html lang="en-us">' in html
