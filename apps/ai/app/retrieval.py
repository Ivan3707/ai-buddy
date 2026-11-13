"""Knowledge-base retrieval.

- Lexical: BM25 over tokens. Two analyzers: crude stems (first STEM_LEN letters, baseline) and
  lemmas via pymorphy3 ("отпуска" -> "отпуск", "стендап" stays apart from "стенд").
- Dense: cosine similarity of sentence embeddings (multilingual model on CPU, ONNX via fastembed).
- Hybrid: Reciprocal Rank Fusion of both rankings.
Every variant is compared on the eval set (recall@k, MRR); see apps/ai/eval/README.md.
"""
from __future__ import annotations

import json
import math
import re
from collections import Counter
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field
from functools import lru_cache
from pathlib import Path
from typing import Protocol

TOKEN = re.compile(r"\w+", re.UNICODE)
STEM_LEN = 5
# Function words carry no topic. Without this list a tiny KB makes "на" look rare, hence "important".
STOPWORDS = frozenset("""
а без бы был была были было в во вот все всё вы где да для до его ее её если есть еще ещё же за и из или
им их к как какая какие каким какой ко когда кто ли либо мне мой мы на над не нет ни но ну о об обо от по
под при про с со так также там те то того тоже ты у уже чем что чтобы эта эти это этот я
""".split())


def _words(text: str) -> list[str]:
    return [w for w in TOKEN.findall(text.casefold().replace("ё", "е")) if len(w) > 1 and w not in STOPWORDS]


def tokenize(text: str) -> list[str]:
    """Baseline analyzer: crude stems."""
    return [w[:STEM_LEN] for w in _words(text)]


class LemmaTokenizer:
    """Dictionary lemmatizer: every word form -> its normal form (OpenCorpora dictionary)."""

    def __init__(self) -> None:
        import pymorphy3  # heavy import, only when this analyzer is used
        morph = pymorphy3.MorphAnalyzer()
        self._lemma = lru_cache(maxsize=50_000)(lambda w: morph.parse(w)[0].normal_form.replace("ё", "е"))

    def __call__(self, text: str) -> list[str]:
        return [self._lemma(w) for w in _words(text)]


@dataclass(frozen=True)
class Doc:
    id: str
    title: str
    url: str
    text: str
    keywords: tuple[str, ...] = field(default=())
    updated: str = ""


@dataclass(frozen=True)
class Hit:
    doc: Doc
    score: float
    lexical: float = 0.0  # BM25 score, kept for hybrid hits too (the mock uses it as confidence)


class Retriever(Protocol):
    def search(self, query: str, k: int, min_score: float) -> list[Hit]: ...


def load_kb(path: Path) -> list[Doc]:
    items = json.loads(path.read_text(encoding="utf-8"))
    return [Doc(id=i["id"], title=i["title"], url=i["url"], text=i["text"],
                keywords=tuple(i.get("keywords", ()))) for i in items]


def _front_matter(raw: str) -> tuple[dict[str, str], str]:
    if not raw.startswith("---\n"):
        return {}, raw
    head, _, body = raw[4:].partition("\n---\n")
    meta = dict(line.split(":", 1) for line in head.splitlines() if ":" in line)
    return {k.strip(): v.strip() for k, v in meta.items()}, body


def load_markdown_kb(directory: Path) -> list[Doc]:
    """One .md per document; every '## ' section becomes a chunk (Doc) that keeps the document id.

    Chunking matters: the model gets the relevant section, not a whole page, and BM25 length
    normalisation works on comparable pieces.
    """
    chunks: list[Doc] = []
    for path in sorted(directory.glob("*.md")):
        meta, body = _front_matter(path.read_text(encoding="utf-8").replace("\r\n", "\n"))
        if "id" not in meta:
            continue
        heading, lines = "", []
        sections: list[tuple[str, str]] = []
        for line in body.splitlines() + ["## "]:  # sentinel flushes the last section
            if line.startswith("## "):
                if "".join(lines).strip():
                    sections.append((heading, "\n".join(lines).strip()))
                heading, lines = line[3:].strip(), []
            else:
                lines.append(line)
        for heading, text in sections:
            title = f"{meta['title']} — {heading}" if heading else meta["title"]
            chunks.append(Doc(id=meta["id"], title=title, url=meta["url"], text=text,
                              updated=meta.get("updated", "")))
    return chunks


class BM25Retriever:
    """Okapi BM25: score = sum over query terms of idf * tf*(k1+1) / (tf + k1*(1-b+b*len/avg_len))."""

    def __init__(self, docs: list[Doc], tokenizer: Callable[[str], list[str]] = tokenize,
                 k1: float = 1.5, b: float = 0.75):
        self.docs = docs
        self.tokenize = tokenizer
        self.k1, self.b = k1, b
        self.tfs = [Counter(tokenizer(" ".join((d.title, d.text, *d.keywords)))) for d in docs]
        self.lens = [sum(tf.values()) for tf in self.tfs]
        self.avg_len = (sum(self.lens) / len(docs)) if docs else 0.0
        df = Counter(term for tf in self.tfs for term in tf)
        n = len(docs)
        self.idf = {t: math.log(1 + (n - c + 0.5) / (c + 0.5)) for t, c in df.items()}

    def score(self, query_terms: set[str], i: int) -> float:
        tf, total = self.tfs[i], 0.0
        norm = self.k1 * (1 - self.b + self.b * self.lens[i] / self.avg_len)
        for term in query_terms:
            f = tf.get(term, 0)
            if f:
                total += self.idf[term] * f * (self.k1 + 1) / (f + norm)
        return total

    def search(self, query: str, k: int, min_score: float) -> list[Hit]:
        terms = set(self.tokenize(query))
        if not terms or not self.docs:
            return []
        hits = [Hit(d, sc, sc) for i, d in enumerate(self.docs) for sc in (self.score(terms, i),)]
        hits = [h for h in hits if h.score >= min_score]
        hits.sort(key=lambda h: h.score, reverse=True)
        return hits[:k]


Embedder = Callable[[list[str]], Sequence[Sequence[float]]]


def _unit(v: Sequence[float]) -> list[float]:
    norm = math.sqrt(sum(x * x for x in v)) or 1.0
    return [x / norm for x in v]


class DenseRetriever:
    """Semantic search: embed chunks once at startup, embed the query, rank by cosine similarity."""

    def __init__(self, docs: list[Doc], embed: Embedder, query_prefix: str = "", passage_prefix: str = ""):
        # e5-family models are trained with "query: " / "passage: " prefixes and work worse without them
        self.docs, self.embed, self.query_prefix = docs, embed, query_prefix
        texts = [f"{passage_prefix}{d.title}. {d.text}" for d in docs]
        self.vectors = [_unit(v) for v in embed(texts)] if docs else []

    def search(self, query: str, k: int, min_score: float) -> list[Hit]:
        if not self.docs or not query.strip():
            return []
        q = _unit(self.embed([self.query_prefix + query])[0])
        hits = [Hit(d, sum(a * b for a, b in zip(q, v))) for d, v in zip(self.docs, self.vectors)]
        hits = [h for h in hits if h.score >= min_score]
        hits.sort(key=lambda h: h.score, reverse=True)
        return hits[:k]


class HybridRetriever:
    """Reciprocal Rank Fusion: score = sum over rankers of 1 / (rrf_k + rank).

    Uses ranks, not raw scores, so BM25 (0..15) and cosine (-1..1) need no calibration.
    min_score is not applied: the LLM decides whether the fragments answer the question.
    """

    def __init__(self, lexical: BM25Retriever, dense: DenseRetriever, rrf_k: int = 60, depth: int = 20):
        self.lexical, self.dense, self.rrf_k, self.depth = lexical, dense, rrf_k, depth

    def search(self, query: str, k: int, min_score: float) -> list[Hit]:
        fused: dict[int, float] = {}
        lex_score: dict[int, float] = {}
        for ranking in (self.lexical.search(query, self.depth, 1e-9), self.dense.search(query, self.depth, -1.0)):
            for rank, h in enumerate(ranking, 1):
                key = id(h.doc)
                fused[key] = fused.get(key, 0.0) + 1.0 / (self.rrf_k + rank)
                if h.lexical:
                    lex_score[key] = h.lexical
        by_key = {id(d): d for d in self.lexical.docs}
        hits = [Hit(by_key[key], sc, lex_score.get(key, 0.0)) for key, sc in fused.items()]
        hits.sort(key=lambda h: h.score, reverse=True)
        return hits[:k]


class FastEmbedder:  # pragma: no cover - needs the model files; exercised by eval and the Docker image
    """ONNX sentence embeddings on CPU (no torch, no GPU)."""

    # Models fastembed does not list but that publish an ONNX export on Hugging Face: (dim, onnx file)
    CUSTOM = {"intfloat/multilingual-e5-small": (384, "onnx/model.onnx"),
              "intfloat/multilingual-e5-base": (768, "onnx/model.onnx")}

    def __init__(self, model_name: str):
        from fastembed import TextEmbedding
        if model_name in self.CUSTOM and model_name not in {m["model"] for m in TextEmbedding.list_supported_models()}:
            from fastembed.common.model_description import ModelSource, PoolingType
            dim, onnx_file = self.CUSTOM[model_name]
            TextEmbedding.add_custom_model(model=model_name, pooling=PoolingType.MEAN, normalization=True,
                                           sources=ModelSource(hf=model_name), dim=dim, model_file=onnx_file)
        self.model = TextEmbedding(model_name)

    def __call__(self, texts: list[str]) -> list[list[float]]:
        return [v.tolist() for v in self.model.embed(texts)]


def build_retriever(docs: list[Doc], mode: str, embedding_model: str) -> Retriever:
    """bm25 = lexical only (no model files); hybrid = BM25 + dense embeddings fused by RRF (default)."""
    lexical = BM25Retriever(docs)
    if mode == "bm25":
        return lexical
    prefixes = ("query: ", "passage: ") if "e5" in embedding_model else ("", "")
    return HybridRetriever(lexical, DenseRetriever(docs, FastEmbedder(embedding_model), *prefixes))


def fuse(rankings: list[list[Hit]], k: int, rrf_k: int = 60) -> list[Hit]:
    """Reciprocal Rank Fusion of several rankings of the same chunks; keeps the best lexical score."""
    fused: dict[int, float] = {}
    best: dict[int, Hit] = {}
    for ranking in rankings:
        for rank, h in enumerate(ranking, 1):
            key = id(h.doc)
            fused[key] = fused.get(key, 0.0) + 1.0 / (rrf_k + rank)
            if key not in best or h.lexical > best[key].lexical:
                best[key] = h
    hits = [Hit(best[key].doc, score, best[key].lexical) for key, score in fused.items()]
    hits.sort(key=lambda h: h.score, reverse=True)
    return hits[:k]
