#!/usr/bin/env python3
"""Fill missing locale strings using the public Google clients5 endpoint.

Run from GitHub Actions (fresh egress IP). The script only fills keys whose
translation is absent or still equals the English source, and writes the locale
JSON files in place.
"""
import json
import re
import sys
import time
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LOCALES = ROOT / "web" / "src" / "locales"
TARGETS = {
    "zh": "zh-CN",
    "es": "es",
    "hi": "hi",
    "ar": "ar",
    "pt": "pt",
    "bn": "bn",
    "ru": "ru",
    "ja": "ja",
    "fr": "fr",
}
PH = re.compile(r"\{([a-zA-Z0-9_]+)\}")
PH_BACK = re.compile(r"__\s*PH\s*(\d+)\s*__")
TECH = ["OpenID Connect", "OIDC", "OAuth 2.0", "OAuth", "PKCE", "JWKS", "RS256", "JWT", "MFA", "TOTP", "SSO", "UCenter", "Argon2id", "Google Authenticator", "1Password", "Authy", "SDK"]


def protect(text: str):
    values = []

    def repl(m):
        values.append(m.group(1))
        return f"__PH{len(values) - 1}__"

    out = PH.sub(repl, text)
    for i, term in enumerate(TECH):
        out = out.replace(term, f"__T{i}__")
    return out, values


def restore(text: str, values):
    text = PH_BACK.sub(lambda m: "{" + values[int(m.group(1))] + "}", text)
    for i, term in enumerate(TECH):
        for variant in (f"__T{i}__", f"__ T{i} __", f"__T{i} __", f"__ T{i}__"):
            text = text.replace(variant, term)
    return text.replace("__", "")


def translate(text: str, target: str) -> str:
    url = "https://clients5.google.com/translate_a/t?" + urllib.parse.urlencode(
        {"client": "dict-chrome-ex", "sl": "en", "tl": target, "q": text}
    )
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        data = json.load(resp)
    if isinstance(data, list) and data and isinstance(data[0], str):
        return data[0]
    if isinstance(data, str):
        return data
    raise RuntimeError(f"unexpected translator response: {data!r}")


def translate_batch(items, target):
    protected, all_values = [], []
    for item in items:
        p, vals = protect(item)
        protected.append(p)
        all_values.append(vals)
    joined = " ||| ".join(protected)
    last = None
    for attempt in range(6):
        try:
            out = translate(joined, target)
            parts = [p.strip() for p in out.split("|||")]
            if len(parts) == len(items):
                return [restore(p, v).strip() for p, v in zip(parts, all_values)]
            raise RuntimeError(f"delimiter mismatch {len(parts)} != {len(items)}")
        except Exception as exc:  # noqa: BLE001
            last = exc
            time.sleep(15 + attempt * 15)
    raise RuntimeError(str(last))


def main():
    en = json.loads((LOCALES / "en.json").read_text())
    keys = list(en.keys())
    overall = 0
    for code, target in TARGETS.items():
        path = LOCALES / f"{code}.json"
        existing = json.loads(path.read_text()) if path.exists() else {}
        todo = [k for k in keys if not existing.get(k) or existing.get(k) == k]
        if not todo:
            print(f"{code}: up to date", flush=True)
            continue
        print(f"{code}: {len(todo)} missing", flush=True)
        result = dict(existing)
        batch, size = [], 0

        def flush():
            nonlocal batch, size
            if not batch:
                return
            try:
                translated = translate_batch(batch, target)
                for src, value in zip(batch, translated):
                    if value:
                        result[src] = value
            except Exception as exc:  # noqa: BLE001
                print(f"{code}: batch failed ({exc}); translating individually", flush=True)
                for src in batch:
                    try:
                        value = translate(protect(src)[0], target)
                        vals = protect(src)[1]
                        value = restore(value, vals).strip()
                        if value:
                            result[src] = value
                    except Exception as inner:  # noqa: BLE001
                        print(f"{code}: skipped {src!r}: {inner}", flush=True)
                    time.sleep(0.3)
            path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
            batch, size = [], 0
            time.sleep(0.5)

        for key in todo:
            if batch and (size + len(key) > 900 or len(batch) >= 20):
                flush()
            batch.append(key)
            size += len(key) + 5
        flush()
        missing = sum(1 for k in keys if result.get(k, k) == k)
        overall += missing
        print(f"{code}: written, still English: {missing}", flush=True)
    print(f"done; remaining English across locales: {overall}", flush=True)


if __name__ == "__main__":
    sys.exit(main())
