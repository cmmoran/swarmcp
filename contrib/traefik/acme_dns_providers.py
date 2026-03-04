#!/usr/bin/env python3
from __future__ import annotations

import datetime as _dt
import html as _html
import os
import re
import sys
import urllib.request


BASE_URL = "https://go-acme.github.io/lego/dns/"


def _fetch(url: str) -> str:
    with urllib.request.urlopen(url, timeout=30) as resp:
        return resp.read().decode("utf-8", errors="replace")


def _extract_provider_slugs(index_html: str) -> list[str]:
    links = re.findall(r'href="(/lego/dns/[^"/]+/)"', index_html)
    slugs: set[str] = set()
    for link in links:
        parts = [p for p in link.split("/") if p]
        if len(parts) >= 3 and parts[0] == "lego" and parts[1] == "dns":
            slugs.add(parts[2])
    return sorted(slugs)


def _strip_tags(text: str) -> str:
    text = re.sub(r"<[^>]+>", " ", text)
    text = _html.unescape(text)
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def _extract_credentials(provider_html: str) -> dict[str, str]:
    split = re.split(r'id="credentials"', provider_html, maxsplit=1, flags=re.I)
    if len(split) < 2:
        return {}
    section = split[1]
    section = re.split(r"<h2[^>]*>", section, maxsplit=1, flags=re.I)[0]
    table_match = re.search(r"<table.*?</table>", section, re.S | re.I)
    if not table_match:
        return {}
    table_html = table_match.group(0)
    rows = re.findall(r"<tr>(.*?)</tr>", table_html, re.S | re.I)
    creds: dict[str, str] = {}
    for row in rows:
        cells = re.findall(r"<t[dh][^>]*>(.*?)</t[dh]>", row, re.S | re.I)
        if len(cells) < 2:
            continue
        env_match = re.search(r"<code>([^<]+)</code>", cells[0], re.S | re.I)
        if not env_match:
            continue
        env = _strip_tags(env_match.group(1))
        desc = _strip_tags(cells[1])
        if env:
            creds[env] = desc
    return creds


def _extract_example_sets(provider_html: str) -> list[list[str]]:
    pre = re.search(r"<pre[^>]*>(.*?)</pre>", provider_html, re.S | re.I)
    if not pre:
        return []
    code = _strip_tags(pre.group(1))
    code = re.sub(r"\s+#\s*or\s+", "\n# or\n", code, flags=re.I)
    lines = code.splitlines()
    sets: list[list[str]] = []
    current: list[str] = []
    for line in lines:
        stripped = line.strip()
        if re.match(r"#\s*or\b", stripped, re.I):
            if current:
                sets.append(current)
                current = []
            continue
        for env in re.findall(r"([A-Z][A-Z0-9_]+)\s*=", line):
            if env not in current:
                current.append(env)
        if "lego " in line:
            if current:
                sets.append(current)
                current = []
    if current:
        sets.append(current)
    return sets


def _write_yaml(out_path: str, providers: dict[str, dict[str, object]]) -> None:
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        stamp = _dt.datetime.now(_dt.UTC).replace(microsecond=0).isoformat()
        f.write("# Generated from %s on %s\n" % (BASE_URL, stamp))
        f.write("providers:\n")

        def q(val: str) -> str:
            escaped = val.replace("\\", "\\\\").replace('"', '\\"')
            return '"' + escaped + '"'

        for provider in sorted(providers.keys()):
            envs = providers[provider]
            f.write("  %s:\n" % provider)
            f.write("    required_env:\n")
            required_sets = envs.get("required_env", [])
            for req_set in required_sets:
                first = True
                for env, desc in req_set.items():
                    if first:
                        f.write("      - %s: %s\n" % (env, q(desc)))
                        first = False
                    else:
                        f.write("        %s: %s\n" % (env, q(desc)))
            f.write("    optional_env:\n")
            optional_list = envs.get("optional_env", [])
            for item in optional_list:
                for env, desc in item.items():
                    f.write("      - %s: %s\n" % (env, q(desc)))


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: %s <output.yaml>" % sys.argv[0], file=sys.stderr)
        return 2
    out_path = sys.argv[1]
    index_html = _fetch(BASE_URL)
    slugs = _extract_provider_slugs(index_html)
    providers: dict[str, dict[str, object]] = {}
    for slug in slugs:
        url = BASE_URL + slug + "/"
        try:
            html = _fetch(url)
        except Exception:
            continue
        creds = _extract_credentials(html)
        example_sets = _extract_example_sets(html)
        required_sets: list[dict[str, str]] = []
        for env_list in example_sets:
            req: dict[str, str] = {}
            for env in env_list:
                req[env] = creds.get(env, "Unknown")
            if req:
                required_sets.append(req)
        used = {env for envs in required_sets for env in envs.keys()}
        optional_list: list[dict[str, str]] = []
        for env, desc in creds.items():
            if env in used:
                continue
            optional_list.append({env: desc})
        if required_sets or optional_list:
            providers[slug] = {
                "required_env": required_sets,
                "optional_env": optional_list,
            }
    _write_yaml(out_path, providers)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
