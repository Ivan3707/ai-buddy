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


@dataclass(frozen=True)
class Hit:
    doc: Doc
    score: float


def load_kb(path: Path) -> list[Doc]:
    items = json.loads(path.read_text(encoding="utf-8"))
    return [Doc(id=i["id"], title=i["title"], url=i["url"], text=i["text"],
                keywords=tuple(i.get("keywords", ()))) for i in items]


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
