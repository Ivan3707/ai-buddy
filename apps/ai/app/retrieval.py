"""Knowledge-base retrieval: BM25 over crude stems.

Baseline on purpose: tokens are cut to their first STEM_LEN letters ("отпуска" -> "отпус"),
a poor man's stemmer for Russian. Lemmatization (pymorphy3) and embeddings come next and must
beat this baseline on the eval set (recall@k) to be kept.
"""
from __future__ import annotations

import json
import math
import re
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path

TOKEN = re.compile(r"\w+", re.UNICODE)
STEM_LEN = 5
# Function words carry no topic. Without this list a tiny KB makes "на" look rare, hence "important".
STOPWORDS = frozenset("""
а без бы был была были было в во вот все всё вы где да для до его ее её если есть еще ещё же за и из или
им их к как какая какие каким какой ко когда кто ли либо мне мой мы на над не нет ни но ну о об обо от по
под при про с со так также там те то того тоже ты у уже чем что чтобы эта эти это этот я
""".split())


def tokenize(text: str) -> list[str]:
    words = TOKEN.findall(text.casefold().replace("ё", "е"))
    return [w[:STEM_LEN] for w in words if len(w) > 1 and w not in STOPWORDS]


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

    def __init__(self, docs: list[Doc], k1: float = 1.5, b: float = 0.75):
        self.docs = docs
        self.k1, self.b = k1, b
        self.tfs = [Counter(tokenize(" ".join((d.title, d.text, *d.keywords)))) for d in docs]
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
        terms = set(tokenize(query))
        if not terms or not self.docs:
            return []
        hits = [Hit(d, self.score(terms, i)) for i, d in enumerate(self.docs)]
        hits = [h for h in hits if h.score >= min_score]
        hits.sort(key=lambda h: h.score, reverse=True)
        return hits[:k]
