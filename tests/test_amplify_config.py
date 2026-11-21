"""The amplify.yml files `photo init --amplify` writes into new sites."""
from pathlib import Path

import pytest
import yaml

FILES = Path(__file__).resolve().parents[1] / "tool" / "internal" / "scaffold" / "files"
VARIANTS = {"theme copy": FILES / "amplify.yml", "module": FILES / "amplify-module.yml"}


def _spec(path):
    return yaml.safe_load(path.read_text())


def _cmds(spec, phase):
    return " ".join(spec["frontend"]["phases"][phase]["commands"])


@pytest.mark.parametrize("path", VARIANTS.values(), ids=VARIANTS.keys())
def test_builds_with_pinned_hugo_extended(path):
    spec = _spec(path)
    assert spec["version"] == 1
    assert spec["frontend"]["artifacts"]["baseDirectory"] == "public"
    pre = _cmds(spec, "preBuild")
    assert "HUGO_VERSION=0.166.0" in pre
    assert "hugo_extended_" in pre
    assert "--minify" in _cmds(spec, "build")


def test_theme_copy_variant_needs_no_go():
    spec = _spec(VARIANTS["theme copy"])
    assert "go.dev/dl" not in _cmds(spec, "preBuild")
    assert "/tmp/go" not in _cmds(spec, "build")


def test_module_variant_installs_go():
    spec = _spec(VARIANTS["module"])
    assert "go${GO_VERSION}.linux-amd64.tar.gz" in _cmds(spec, "preBuild")
    assert "/tmp/go/bin" in _cmds(spec, "build")
