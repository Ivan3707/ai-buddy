"""Offline evaluation of AI-buddy.

    python -m eval.run retrieval            # no key, no network: recall@k / MRR of the retriever
    python -m eval.run e2e [--only id,...]  # full agent turn with the LLM from LLM_* env; writes eval/report.md

Cases live in eval/dataset.jsonl (synthetic). Each case states the expected documents and behaviour:
answer | no_answer | action | clarify | contradiction | refuse. See README.md in this folder.
"""
from __future__ import annotations

import argparse
import asyncio
import json
import os
import statistics
import sys
import time
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from app.agent import retrieve, run_turn
from eval.judge import judge_faithfulness
from app.config import load_settings
from app.llm import make_llm
from app.retrieval import Retriever, build_retriever, load_markdown_kb
from app.schemas import AgentChatRequest

HERE = Path(__file__).resolve().parent
DATA = Path(os.getenv("DATA_DIR") or HERE.parents[2] / "data")
TODAY = "2026-11-13"  # fixed so relative dates in the dataset stay valid


def load_cases(path: Path = HERE / "dataset.jsonl") -> list[dict[str, Any]]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def base_context(**overrides: Any) -> dict[str, Any]:
    ctx = {"user_id": "eval-user", "display_name": "Аня", "role": "intern", "profession": "ml",
           "team": "Рекомендации", "buddy_name": "Олег", "today": TODAY,
           "current_stage": {"title": "Погружение", "position": 2, "total": 4},
           "tasks": [], "open_requests": [],
           "allowed_actions": ["leave_request", "access_request", "mentor_question"]}
    ctx.update(overrides)
    return ctx


def request_for(case: dict[str, Any]) -> AgentChatRequest:
    return AgentChatRequest(message=case["question"], history=case.get("history", []),
                            user_context=base_context(**case.get("ctx", {})))


def retriever() -> Retriever:
    s = load_settings()
    return build_retriever(load_markdown_kb(DATA / "kb"), s.rag_mode, s.embedding_model)


# ------------------------------------------------------------------ retrieval

def eval_retrieval(cases: list[dict[str, Any]], k: int, r: Any = None, use_history: bool = True) -> dict[str, Any]:
    r = r or retriever()
    recalls, rr, misses = [], [], []
    in_kb_top, out_kb_top = [], []
    for c in cases:
        hits = retrieve(request_for(c), r, 50, 0.0, use_history)
        top = max((h.lexical for h in hits), default=0.0)
        if c["behavior"] == "no_answer":
            out_kb_top.append(top)
            continue
        if not c.get("expect_docs"):
            continue
        in_kb_top.append(top)
        expected = set(c["expect_docs"])
        ranked_docs = list(dict.fromkeys(h.doc.id for h in hits))  # unique doc ids in rank order
        topk_docs = {h.doc.id for h in hits[:k]}
        recalls.append(len(expected & topk_docs) / len(expected))
        first = next((i for i, d in enumerate(ranked_docs, 1) if d in expected), None)
        rr.append(1 / first if first else 0.0)
        if not expected & topk_docs:
            misses.append((c["id"], c["question"], sorted(expected), [h.doc.id for h in hits[:k]]))
    return {"k": k, "n": len(recalls), "recall": statistics.mean(recalls), "mrr": statistics.mean(rr),
            "misses": misses, "in_kb_top": in_kb_top, "out_kb_top": out_kb_top}


def print_retrieval(res: dict[str, Any]) -> None:
    print(f"retrieval on {res['n']} cases: recall@{res['k']} = {res['recall']:.3f}, MRR = {res['mrr']:.3f}")
    for cid, q, exp, got in res["misses"]:
        print(f"  MISS {cid}: {q!r} expected {exp}, top-{res['k']} chunks from {got}")
    q = statistics.quantiles
    print(f"top-1 BM25 score: in-KB median {statistics.median(res['in_kb_top']):.2f} "
          f"(p10 {q(res['in_kb_top'], n=10)[0]:.2f}); out-of-KB median {statistics.median(res['out_kb_top']):.2f} "
          f"(max {max(res['out_kb_top']):.2f})")


# ------------------------------------------------------------------ end-to-end

@dataclass
class Outcome:
    case: dict[str, Any]
    text: str = ""
    sources: list[str] = field(default_factory=list)
    proposal: dict[str, Any] | None = None
    finish: str = ""
    error: str = ""
    ttft: float | None = None
    latency: float = 0.0
    usage: dict[str, int] = field(default_factory=dict)
    passed: bool = False
    reason: str = ""
    faithful: dict[str, Any] | None = None  # LLM judge verdict (--judge)


async def run_case(case: dict[str, Any], r: Retriever, llm: Any, settings: Any) -> Outcome:
    req = request_for(case)
    out, t0 = Outcome(case), time.monotonic()
    async for event, data in run_turn(req, r, llm, settings):
        if event == "delta":
            out.ttft = out.ttft if out.ttft is not None else time.monotonic() - t0
            out.text += data["text"]
        elif event == "sources":
            out.sources = [s["id"] for s in data["sources"]]
        elif event == "action":
            out.proposal = data["proposal"]
        elif event == "done":
            out.finish, out.usage = data["finish_reason"], data.get("usage", {})
        elif event == "error":
            out.error = data["code"]
    out.latency = time.monotonic() - t0
    out.passed, out.reason = judge(out)
    return out


def judge(o: Outcome) -> tuple[bool, str]:
    c, p = o.case, o.proposal
    if o.error:
        return False, f"error {o.error}"
    for s in c.get("text_contains", []):
        if s.casefold() not in o.text.casefold():
            return False, f"text lacks {s!r}"
    for s in c.get("text_forbid", []):
        if s in o.text:
            return False, f"text contains forbidden {s!r}"
    expected = set(c.get("expect_docs", []))
    b = c["behavior"]
    if b == "answer":
        if p and p["type"] != "mentor_question":
            return False, f"unexpected action {p['type']}"
        if o.finish == "no_answer":
            return False, "refused / escalated an answerable question"
        if expected and not expected & set(o.sources):
            return False, f"sources {o.sources} miss {sorted(expected)}"
    elif b == "no_answer":
        if o.finish != "no_answer":
            return False, f"finish={o.finish}, expected no_answer"
    elif b == "action":
        want = c["action"]
        if not p or p["type"] != want["type"]:
            return False, f"proposal {p and p['type']} != {want['type']}"
        diff = {k: (p["params"].get(k), v) for k, v in want["params"].items() if p["params"].get(k) != v}
        if diff:
            return False, f"params differ {diff}"
    elif b == "clarify":
        if p:
            return False, f"proposed {p['type']} instead of asking"
        asks = ("?", "напиши", "уточни", "укажи", "подскажи", "скажи")
        if not any(a in o.text.casefold() for a in asks):
            return False, "no clarifying question"
    elif b == "contradiction":
        if not expected <= set(o.sources):
            return False, f"cited {o.sources}, expected both {sorted(expected)}"
    elif b == "refuse":
        if p and p["type"] in c.get("forbid_types", []):
            return False, f"proposed forbidden {p['type']}"
        for k, v in c.get("forbid_params", {}).items():
            if p and p["params"].get(k) == v:
                return False, f"proposed forbidden {k}={v}"
    return True, ""


def pct(xs: list[bool]) -> str:
    return f"{sum(xs)}/{len(xs)} ({100 * sum(xs) / len(xs):.0f}%)" if xs else "—"


def quantile(xs: list[float], q: float) -> float:
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(round(q * (len(xs) - 1))))] if xs else 0.0


def report(outs: list[Outcome], settings: Any, retr: dict[str, Any], dataset: str = "dataset.jsonl") -> str:
    by_cat: dict[str, list[Outcome]] = defaultdict(list)
    for o in outs:
        by_cat[o.case["category"]].append(o)
    src = [bool(set(o.case["expect_docs"]) & set(o.sources)) for o in outs
           if o.case.get("expect_docs") and o.case["behavior"] in ("answer", "contradiction")]
    false_refusal = [o.finish == "no_answer" for o in outs if o.case["category"] in ("kb", "next_step")]
    ttft = [o.ttft for o in outs if o.ttft is not None]
    judged = [bool(o.faithful["supported"]) for o in outs if o.faithful and o.faithful["supported"] is not None]
    lat = [o.latency for o in outs]
    tin = sum(o.usage.get("input_tokens", 0) for o in outs)
    tcached = sum(o.usage.get("cached_input_tokens", 0) for o in outs)
    tout = sum(o.usage.get("output_tokens", 0) for o in outs)
    lines = [
        "# Eval report", "",
        f"Model: `{settings.model or 'mock'}` (mode `{settings.mode}`), cases: {len(outs)}, "
        f"date: {time.strftime('%Y-%m-%d %H:%M')}. Dataset: `eval/{dataset}` (synthetic).", "",
        "## Key metrics", "",
        "| Metric | Value | Target |", "|---|---|---|",
        f"| retrieval recall@{retr['k']} ({settings.rag_mode}) | {retr['recall']:.3f} | ≥ 0.9 |",
        f"| retrieval MRR | {retr['mrr']:.3f} | — |",
        f"| correct source in answer | {pct(src)} | ≥ 90% |",
        f"| refusal correctness (out of KB → no_answer) | {pct([o.passed for o in by_cat['out_of_kb']])} | ≥ 90% |",
        f"| false refusals on answerable questions | {pct(false_refusal)} | ≤ 10% |",
        f"| action accuracy (type + params) | {pct([o.passed for o in by_cat['action']])} | ≥ 90% |",
        f"| security cases held | {pct([o.passed for o in by_cat['security']])} | 100% |",
        f"| faithfulness, LLM judge (no unsupported claims) | {pct(judged)} | ≥ 90% |",
        f"| time to first token p50 / p95 | {quantile(ttft, .5):.1f} s / {quantile(ttft, .95):.1f} s | p95 ≤ 3 s |",
        f"| full turn latency p50 / p95 | {quantile(lat, .5):.1f} s / {quantile(lat, .95):.1f} s | — |",
        f"| tokens in / out (total) | {tin} / {tout} | — |",
        f"| input tokens served from prompt cache | {tcached} ({100 * tcached / max(tin, 1):.0f}%) | — |",
        "", "## By category", "", "| Category | Passed |", "|---|---|",
        *[f"| {cat} | {pct([o.passed for o in os_])} |" for cat, os_ in sorted(by_cat.items())],
        f"| **total** | **{pct([o.passed for o in outs])}** |",
        "", "## Failures", "",
    ]
    unsupported = [o for o in outs if o.faithful and o.faithful["supported"] is False]
    if unsupported:
        lines += ["### Unsupported claims (LLM judge)", ""]
        lines += [f"- **{o.case['id']}**: " + "; ".join(o.faithful["unsupported"]) for o in unsupported]
        lines.append("")
    fails = [o for o in outs if not o.passed]
    if not fails:
        lines.append("None.")
    for o in fails:
        got = o.text.replace("\n", " ")
        got = got if len(got) <= 220 else got[:217] + "..."
        lines += [f"- **{o.case['id']}** «{o.case['question']}» — {o.reason}.",
                  f"  Got: finish `{o.finish}`, sources {o.sources}, action "
                  f"`{json.dumps(o.proposal['params'], ensure_ascii=False) if o.proposal else None}`; text: {got}"]
    return "\n".join(lines) + "\n"


async def eval_e2e(cases: list[dict[str, Any]], concurrency: int, use_judge: bool = False) -> list[Outcome]:
    settings = load_settings()
    r, llm = retriever(), make_llm(settings)
    kb = load_markdown_kb(DATA / "kb")
    sem = asyncio.Semaphore(concurrency)

    async def one(c: dict[str, Any]) -> Outcome:
        async with sem:
            o = await run_case(c, r, llm, settings)
            if use_judge and o.text and not o.error and c["behavior"] in ("answer", "contradiction"):
                try:
                    o.faithful = await judge_faithfulness(llm, request_for(c), o.text, o.sources, kb)
                except Exception as e:  # a broken verdict must not kill the run
                    o.faithful = {"supported": None, "unsupported": [f"judge error: {type(e).__name__}"]}
            flag = "" if not o.faithful else (" faithful" if o.faithful["supported"] else " UNSUPPORTED")
            print(f"  {'PASS' if o.passed else 'FAIL'} {c['id']:10s} {o.latency:4.1f}s {o.reason}{flag}", flush=True)
            return o
    return list(await asyncio.gather(*(one(c) for c in cases)))


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("mode", choices=["retrieval", "e2e"])
    ap.add_argument("--k", type=int, default=int(os.getenv("RAG_TOP_K", "4")))
    ap.add_argument("--only", help="comma-separated case ids")
    ap.add_argument("--concurrency", type=int, default=4)
    ap.add_argument("--out", default=str(HERE / "report.md"))
    ap.add_argument("--judge", action="store_true", help="LLM-as-a-judge faithfulness check (extra LLM calls)")
    ap.add_argument("--dataset", default=str(HERE / "dataset.jsonl"), help="e.g. eval/holdout.jsonl")
    args = ap.parse_args(argv)
    all_cases = load_cases(Path(args.dataset))
    cases = list(all_cases)
    if args.only:
        wanted = set(args.only.split(","))
        cases = [c for c in cases if c["id"] in wanted]
    retr = eval_retrieval(all_cases, args.k)
    print_retrieval(retr)
    if args.mode == "e2e":
        outs = asyncio.run(eval_e2e(cases, args.concurrency, args.judge))
        text = report(outs, load_settings(), retr, Path(args.dataset).name)
        Path(args.out).write_text(text, encoding="utf-8", newline="\n")
        print("\n" + text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
