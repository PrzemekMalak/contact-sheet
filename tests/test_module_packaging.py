"""The theme must survive being packaged as a Go/Hugo module.

Go module zips silently drop every directory named "vendor" (and nested
modules), so a theme file under such a path works from a git checkout but is
missing for sites that import the theme as a Hugo Module.
"""
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
THEME_DIRS = ["layouts", "assets", "content", "static", "data", "i18n", "archetypes"]


def test_no_vendor_directories_in_theme():
    offenders = [
        str(p.relative_to(REPO))
        for d in THEME_DIRS if (REPO / d).is_dir()
        for p in (REPO / d).rglob("vendor") if p.is_dir()
    ]
    assert offenders == [], f"rename these, Go module zips drop them: {offenders}"
