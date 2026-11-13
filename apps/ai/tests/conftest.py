import os

os.environ.setdefault("RAG_MODE", "bm25")  # unit tests never download embedding models

import json

import pytest


def parse_sse(text: str) -> list[tuple[str, dict]]:
    events = []
    for block in text.strip().split("\n\n"):
        fields = dict(line.split(": ", 1) for line in block.splitlines())
        events.append((fields["event"], json.loads(fields["data"])))
    return events


def user_context(**overrides) -> dict:
    ctx = {
        "user_id": "u-1", "display_name": "Аня", "role": "intern", "profession": "ml",
        "team": "Рекомендации", "buddy_name": "Олег", "today": "2026-11-13",
        "current_stage": {"title": "Первая неделя", "position": 1, "total": 4},
        "tasks": [{"id": "t1", "title": "Настроить окружение", "status": "active", "checklist": "common"}],
        "open_requests": [],
        "allowed_actions": ["leave_request", "access_request", "mentor_question"],
    }
    ctx.update(overrides)
    return ctx


@pytest.fixture
def chat_body():
    def make(message: str, **ctx) -> dict:
        return {"message": message, "history": [], "user_context": user_context(**ctx)}
    return make
