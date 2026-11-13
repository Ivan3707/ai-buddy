"""Turn 👎 feedback into eval candidates: every bad answer becomes a regression case.

    python -m eval.from_feedback feedback.jsonl [--out eval/candidates.jsonl]

Input: one JSON object per line, exported by Backend from chat messages with feedback, e.g.
    {"message_id": "…", "question": "…", "answer": "…", "sources": ["leave"],
     "finish_reason": "answered", "rating": "down", "created_at": "…"}
Output: dataset.jsonl-shaped stubs with behavior "TODO" and the original answer kept for the
reviewer. A human sets behavior / expect_docs, then moves the case into dataset.jsonl.
Questions already in dataset.jsonl or holdout.jsonl (case/space-insensitive) are skipped.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent


def normalize(text: str) -> str:
    return re.sub(r"\s+", " ", text.casefold().replace("ё", "е")).strip(" ?!.")


def known_questions() -> set[str]:
    known: set[str] = set()
    for name in ("dataset.jsonl", "holdout.jsonl"):
        path = HERE / name
        if path.exists():
            known |= {normalize(json.loads(line)["question"])
                      for line in path.read_text(encoding="utf-8").splitlines() if line.strip()}
    return known


def candidates(rows: list[dict[str, Any]], known: set[str]) -> list[dict[str, Any]]:
    out, seen = [], set(known)
    for row in rows:
        if row.get("rating") != "down" or not row.get("question"):
            continue
        key = normalize(row["question"])
        if key in seen:
            continue
        seen.add(key)
        out.append({
            "id": f"fb-{len(out) + 1:03d}", "category": "feedback", "question": row["question"],
            "behavior": "TODO", "expect_docs": [],
            "_review": {"answer": row.get("answer", ""), "sources": row.get("sources", []),
                        "finish_reason": row.get("finish_reason"), "message_id": row.get("message_id")},
        })
    return out


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("feedback")
    ap.add_argument("--out", default=str(HERE / "candidates.jsonl"))
    args = ap.parse_args(argv)
    rows = [json.loads(line) for line in Path(args.feedback).read_text(encoding="utf-8").splitlines() if line.strip()]
    result = candidates(rows, known_questions())
    Path(args.out).write_text("".join(json.dumps(c, ensure_ascii=False) + "\n" for c in result),
                              encoding="utf-8", newline="\n")
    print(f"{len(result)} new candidates from {len(rows)} feedback rows -> {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
