#!/usr/bin/env python3
"""Generate locale dictionaries with the public MyMemory translation API.

English strings are the source keys. Run this helper after adding new keys and
review the generated JSON files in web/src/locales before shipping.
"""
import html
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
TECH = ["OpenID Connect", "OIDC", "OAuth 2.0", "OAuth", "PKCE", "JWKS", "RS256", "JWT", "MFA", "TOTP", "SSO", "UCenter", "Argon2id", "Google Authenticator", "1Password", "Authy", "SDK"]
PH = re.compile(r"\{([a-zA-Z0-9_]+)\}")
PH_BACK = re.compile(r"__\s*PH\s*(\d+)\s*__")


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


def translate_batch(text: str, pair: str) -> str:
    url = "https://api.mymemory.translated.net/get?" + urllib.parse.urlencode(
        {"q": text, "langpair": pair}
    )
    req = urllib.request.Request(url, headers={"User-Agent": "UCenter-i18n/1.0"})
    with urllib.request.urlopen(req, timeout=40) as resp:
        data = json.load(resp)
    if data.get("responseStatus") != 200:
        raise RuntimeError(data.get("responseDetails") or f"status {data.get('responseStatus')}")
    out = data["responseData"]["translatedText"]
    if data.get("quotaFinished"):
        raise RuntimeError("translation quota finished")
    if "MYMEMORY WARNING" in out.upper():
        raise RuntimeError(out)
    if out.strip() == text.strip():
        raise RuntimeError("translator returned the source text")
    return html.unescape(out)


def translate_batch_with_retry(text: str, pair: str) -> str:
    last = None
    for attempt in range(5):
        try:
            return translate_batch(text, pair)
        except Exception as exc:  # noqa: BLE001
            last = exc
            time.sleep(4 + attempt * 4)
    raise RuntimeError(str(last))


def main():
    en = json.loads((LOCALES / "en.json").read_text())
    keys = list(en.keys())
    only = sys.argv[1] if len(sys.argv) > 1 else None
    for code, pair in TARGETS.items():
        if only and code != only:
            continue
        target = LOCALES / f"{code}.json"
        result = {}
        if target.exists():
            try:
                result = json.loads(target.read_text())
            except json.JSONDecodeError:
                result = {}
        todo = [k for k in keys if not result.get(k) or result.get(k) == k]
        if not todo:
            print(f"{code}: up to date", flush=True)
            continue
        print(f"{code}: translating {len(todo)} strings", flush=True)
        batch, batch_len = [], 0

        def flush():
            nonlocal batch, batch_len
            if not batch:
                return
            protected, all_values = [], []
            for item in batch:
                p, vals = protect(item)
                protected.append(p)
                all_values.append(vals)
            try:
                translated = translate_batch_with_retry(" ||| ".join(protected), pair)
                parts = [p.strip() for p in translated.split("|||")]
                if len(parts) != len(batch):
                    raise RuntimeError(f"delimiter mismatch {len(parts)} != {len(batch)}")
                for src, part, vals in zip(batch, parts, all_values):
                    value = restore(part, vals).strip()
                    if value:
                        result[src] = value
            except Exception as exc:  # noqa: BLE001
                print(f"  ! batch failed: {exc}", flush=True)
                for src in batch:
                    result.setdefault(src, src)
            target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
            batch, batch_len = [], 0
            time.sleep(1.0)

        for key in todo:
            if batch and (batch_len + len(key) > 420 or len(batch) >= 18):
                flush()
            batch.append(key)
            batch_len += len(key) + 5
        flush()
        missing = sum(1 for k in keys if result.get(k, k) == k)
        print(f"{code}: wrote {len(result)} strings, still English: {missing}", flush=True)


if __name__ == "__main__":
    main()
