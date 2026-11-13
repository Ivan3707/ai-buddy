"""Compare retrieval variants on the eval set (no LLM, no key).

    python -m eval.compare_retrievers
Needs pymorphy3 and fastembed (the embedding model is downloaded on first run, ~0.2 GB).
"""
from __future__ import annotations

import time

from app.retrieval import BM25Retriever, DenseRetriever, FastEmbedder, HybridRetriever, LemmaTokenizer, load_markdown_kb
from eval.run import DATA, eval_retrieval, load_cases

import os

# (model, query prefix, passage prefix)
MODELS = [
    ("sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2", "", ""),
    ("sentence-transformers/paraphrase-multilingual-mpnet-base-v2", "", ""),
    ("intfloat/multilingual-e5-small", "query: ", "passage: "),
    ("intfloat/multilingual-e5-base", "query: ", "passage: "),
    ("intfloat/multilingual-e5-large", "query: ", "passage: "),
]
if os.getenv("MODELS"):
    MODELS = [m for m in MODELS if m[0].split("/")[-1] in os.getenv("MODELS", "").split(",")]


def main() -> None:
    cases, docs = load_cases(), load_markdown_kb(DATA / "kb")
    lemma = LemmaTokenizer()
    variants = {"bm25 stem5 (baseline)": BM25Retriever(docs), "bm25 lemma": BM25Retriever(docs, lemma)}
    for m, qp, pp in MODELS:
        t0 = time.monotonic()
        dense = DenseRetriever(docs, FastEmbedder(m), qp, pp)
        print(f"{m}: indexed {len(docs)} chunks in {time.monotonic() - t0:.1f}s")
        short = m.split("/")[-1]
        variants[f"dense {short}"] = dense
        variants[f"hybrid lemma + {short}"] = HybridRetriever(variants["bm25 lemma"], dense)
        variants[f"hybrid stem5 + {short}"] = HybridRetriever(variants["bm25 stem5 (baseline)"], dense)
    print(f"\n| Retriever | recall@4 | MRR | misses | ms/query |\n|---|---|---|---|---|")
    for name, r in variants.items():
        t0 = time.monotonic()
        res = eval_retrieval(cases, 4, r)
        ms = (time.monotonic() - t0) * 1000 / len(cases)
        print(f"| {name} | {res['recall']:.3f} | {res['mrr']:.3f} | {', '.join(m[0] for m in res['misses']) or '—'} | {ms:.0f} |")


if __name__ == "__main__":
    main()
