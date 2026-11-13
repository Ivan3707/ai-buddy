"""Mask personal data before text reaches retrieval and the LLM provider.

The case forbids processing sensitive personal data; interns still paste it by accident
("мой паспорт 4510 123456, оформи…"). We replace it with a placeholder the model understands.
Only user-written text is masked (message, user turns of history, page selection); the KB is ours.
"""
from __future__ import annotations

import re
from collections import Counter

PLACEHOLDER = {"card": "[номер карты скрыт]", "phone": "[телефон скрыт]", "snils": "[СНИЛС скрыт]",
               "passport": "[паспорт скрыт]"}

# Order matters: longer / more specific patterns first so a card is not half-eaten by the phone rule.
PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("card", re.compile(r"(?<!\d)(?:\d[ -]?){12,18}\d(?!\d)")),
    ("snils", re.compile(r"(?<!\d)\d{3}-\d{3}-\d{3}[ -]\d{2}(?!\d)")),
    ("phone", re.compile(r"(?<![\d+])(?:\+7|8)[ \-(]*\d{3}[ \-)]*\d{3}[ -]*\d{2}[ -]*\d{2}(?!\d)")),
    ("passport", re.compile(r"(?<!\d)\d{2} ?\d{2} ?\d{6}(?!\d)")),
]


def luhn_ok(digits: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(digits)):
        d = int(ch)
        if i % 2:
            d = d * 2 - 9 if d > 4 else d * 2
        total += d
    return total % 10 == 0


def mask(text: str) -> tuple[str, Counter[str]]:
    found: Counter[str] = Counter()

    for kind, pattern in PATTERNS:
        def repl(m: re.Match[str], kind: str = kind) -> str:
            if kind == "card" and not luhn_ok(re.sub(r"\D", "", m.group())):
                return m.group()  # long number that is not a card (order id, timestamp...)
            found[kind] += 1
            return PLACEHOLDER[kind]
        text = pattern.sub(repl, text)
    return text, found
