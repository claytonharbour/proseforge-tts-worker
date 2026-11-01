#!/usr/bin/env python3
"""Measure which tests actually notice a defect.

A green suite says every test passed. It does not say any of them would have
failed. This reintroduces defects we have already fixed, one at a time, runs the
WHOLE unit suite against each, and reports which tests noticed.

    make dev-test-unit                → 309 tests pass
    this                              → and here is how many defend each fix

Two conditions make it work, and both are load-bearing:

  BATCHED      run every assertion against one injection. Red-proofing a single
               test tells you that test discriminates and nothing about its
               neighbours. A decimal test of mine counted TokenizeText groups —
               which regroup adjacent sentences — so it passed with the bug
               restored; only running three together showed one staying green.

  BASELINE     an uninjected run first. Without it "3 failing" might be three
               tests that were already red.

⚠️ STALE-INJECTION GUARD — @crispin's, and this harness needed it.
An injection whose anchor no longer matches applies nothing, the suite passes,
and the row reads "0 tests noticed" — a DEFENDED fix reported as UNDEFENDED.
That is a null instrument inside the instrument for null instruments. Every
injection therefore asserts its anchor appears exactly once and reports STALE
rather than a count. My first version of this had the hole: three injections
asserted their anchor and the fourth used dict.pop(), which no-ops silently.

⛔ LIMIT: this finds unguarded FIXES, never unguarded BEHAVIOUR. A defect nobody
has fixed has no injection here. It is a floor, not a coverage number.
"""

import json
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]


def _text_injection(rel, find, replace):
    """Replace `find` once. Refuses unless it appears exactly once."""

    def apply(path: Path):
        s = path.read_text()
        n = s.count(find)
        if n != 1:
            return f"anchor appears {n} times, expected 1"
        path.write_text(s.replace(find, replace, 1))
        return None

    return rel, apply


def _json_drop_key(rel, sections, key):
    """Remove `key` from each named section. Refuses if it is already absent."""

    def apply(path: Path):
        d = json.loads(path.read_text())
        missing = [s for s in sections if key not in d.get(s, {})]
        if missing:
            return f"{key!r} already absent from {missing}"
        for s in sections:
            d[s].pop(key)
        path.write_text(json.dumps(d, ensure_ascii=False, indent=2))
        return None

    return rel, apply


# (label, ticket, relative path, mutate) — each reintroduces a real shipped fix.
INJECTIONS = [
    ("isPunctuation loses '*'", "#58",
     *_text_injection("internal/phonemizer/phonemizer.go", "\t\t'*':", "\t\t'§':")),
    ("decimal guard removed", "#59",
     *_text_injection("internal/phonemizer/tokenizer.go",
                      " && !isDecimalPoint(runes, i)", "")),
    ("DiagnoseText back to whole-text", "#47",
     *_text_injection(
         "internal/phonemizer/phonemizer.go",
         "\tvar results []WordDiagnosis\n"
         "\tfor _, para := range SplitParagraphs(text) {\n"
         "\t\tfor _, sent := range SplitSentences(para) {\n"
         "\t\t\tresults = append(results, p.diagnoseSentence(sent)...)\n"
         "\t\t}\n"
         "\t}\n"
         "\treturn results\n}",
         "\treturn p.diagnoseSentence(text)\n}")),
    ("AX dropped from the ARPABET map", "#53",
     *_json_drop_key("data/arpabet_to_ipa.json", ["vowels", "combined"], "AX")),
]

FAIL_RE = re.compile(r"^\s*--- FAIL: (\S+)", re.M)


def run_suite():
    p = subprocess.run(["make", "dev-test-unit", "VERBOSE=1"],
                       cwd=REPO, capture_output=True, text=True)
    return p.returncode, sorted(set(FAIL_RE.findall(p.stdout + p.stderr)))


def main():
    print("BASELINE")
    rc, fails = run_suite()
    if rc != 0 or fails:
        print(f"  ⛔ suite is not green (exit {rc}, {len(fails)} failing) — "
              f"every row below would be unreadable. Fix that first.")
        for f in fails:
            print(f"     {f}")
        return 1
    print("  exit 0, 0 failing — rows below are readable\n")

    rows = []
    for label, ticket, rel, mutate in INJECTIONS:
        path = REPO / rel
        with tempfile.NamedTemporaryFile(delete=False) as bk:
            shutil.copy(path, bk.name)
        try:
            why = mutate(path)
            if why is not None:
                print(f"{label} ({ticket})\n  ⚠️ STALE INJECTION — {why}")
                print("     NOT 'undefended'. The injection did not apply.\n")
                rows.append((label, ticket, None))
                continue
            rc, fails = run_suite()
            noticed = len(fails)
            print(f"{label} ({ticket})\n  exit {rc}, {noticed} test(s) noticed")
            for f in fails:
                print(f"     {f}")
            if noticed == 0:
                print("     ⛔ NOTHING NOTICED — this fix has no guard.")
            print()
            rows.append((label, ticket, noticed))
        finally:
            shutil.copy(bk.name, path)
            Path(bk.name).unlink(missing_ok=True)

    subprocess.run(["go", "build", "./..."], cwd=REPO, check=False)
    print("SUMMARY (a floor: unguarded fixes, never unguarded behaviour)")
    for label, ticket, n in rows:
        print(f"  {ticket:5} {label:38} {'STALE' if n is None else n}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
