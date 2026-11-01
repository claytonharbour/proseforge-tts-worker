#!/usr/bin/env bash
# Inject-and-restore for red-proofs, in a SHARED tree, without destroying anyone's work.
#
#   redproof.sh snapshot <file>    take a backup + record the pre-injection hash
#   redproof.sh restore  <file>    restore ONLY if the file still holds what I injected
#
# 🛑 WHY NEITHER OBVIOUS FORM IS SAFE. Four of us converged on this in one evening
# (proseforge devstory, 2026-08-21) and each form loses something different:
#
#   git checkout -- <file>   restores to HEAD, so it destroys YOUR OWN uncommitted
#                            work in that file. @Tate lost an rc=12 fix this way.
#   cp /tmp/bak <file>       restores the pre-injection state, so your work survives —
#                            but it BLIND-OVERWRITES whatever is there now, taking a
#                            colleague's concurrent edit with it. @Tate named this
#                            about his own form rather than letting it travel.
#
# ⚑ Both are the same hole @Aldric identified: git cannot tell you whose lines are in
# a modified file, and a check before the act reopens a window before the act happens.
# A clean-tree precondition does not close it either — I proposed that and withdrew it,
# because the precondition is verified at the start and relied on at the end, which
# makes it a stale check curing a stale check.
#
# ✅ WHAT ACTUALLY CLOSES IT: do not ask whose work is in the file. Ask whether the
# file still contains exactly what I put there. If it does, nobody touched it and the
# restore is safe. If it does not, someone did — REFUSE and say so, rather than
# picking a winner between their edit and mine.
#
# 📌 The residual window is between the hash check and the copy — microseconds, versus
# the seconds-to-minutes of an inject-run-restore cycle. Not zero. Stated rather than
# implied, because every guard in this fleet that claimed zero was wrong about it.
set -uo pipefail

DIR="${TMPDIR:-/tmp}/redproof"
mkdir -p "$DIR"
key() { printf '%s' "$1" | shasum | cut -d' ' -f1; }

cmd="${1:?usage: redproof.sh snapshot|restore <file>}"
f="${2:?usage: redproof.sh snapshot|restore <file>}"
[ -f "$f" ] || { echo "redproof: no such file: $f" >&2; exit 2; }
k="$DIR/$(key "$f")"

case "$cmd" in
  snapshot)
    cp "$f" "$k.bak"
    echo "redproof: snapshot of $f taken"
    echo "  now make your injection, then run: $0 restore $f"
    ;;
  arm)
    # Record the hash of the file AS INJECTED. Run after injecting.
    shasum "$f" | cut -d' ' -f1 > "$k.injected"
    echo "redproof: armed — restore will refuse unless $f still matches this state"
    ;;
  restore)
    [ -f "$k.bak" ] || { echo "redproof: no snapshot for $f — refusing" >&2; exit 2; }
    if [ -f "$k.injected" ]; then
      now=$(shasum "$f" | cut -d' ' -f1)
      want=$(cat "$k.injected")
      if [ "$now" != "$want" ]; then
        echo "🛑 redproof: REFUSING to restore $f" >&2
        echo "   The file changed since I injected. Someone else's edit is in it, and" >&2
        echo "   overwriting would take their work. Resolve by hand." >&2
        echo "   injected: $want" >&2
        echo "   now:      $now" >&2
        exit 1
      fi
    fi
    cp "$k.bak" "$f"
    rm -f "$k.injected"
    echo "redproof: $f restored to its pre-injection state"
    ;;
  *) echo "usage: $0 snapshot|arm|restore <file>" >&2; exit 2 ;;
esac
