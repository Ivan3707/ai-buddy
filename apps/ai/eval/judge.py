"""LLM-as-a-judge for faithfulness: is every claim of the answer supported by the cited KB documents?

Rule checks (eval.run.judge) see sources and substrings; they cannot see a made-up detail inside a
correct-looking answer. The judge gets the answer plus the full text of the cited documents (and the
user's own context, which is also a legitimate source) and must list unsupported claims as JSON.
Used with `python -m eval.run e2e --judge`; costs about one extra LLM call per answered case.
"""
from __future__ import annotations

import json
import re
from typing import Any

from app.llm import LLM, TextDelta, Turn
from app.retrieval import Doc
from app.schemas import AgentChatRequest

JUDGE_PROMPT = """Ты — строгий проверяющий ответов корпоративного ассистента.
Тебе дают ВОПРОС пользователя, ОТВЕТ ассистента, ДОКУМЕНТЫ базы знаний (с датами обновления) и КОНТЕКСТ
пользователя (его этап, задачи, заявки; buddy_name — имя его наставника, today — сегодняшняя дата).
То, что пользователь сам сообщил в вопросе, тоже считается известным.
Разбей ответ на фактические утверждения о правилах, процессах, сроках, людях, системах и данных пользователя.
Утверждение подтверждено, если оно прямо следует из документов или контекста. Общие советы без фактов
(«напиши, если будут вопросы», «могу оформить заявку») не проверяй. Предложение эскалации — не утверждение.
Верни ТОЛЬКО JSON без пояснений: {"supported": true|false, "unsupported": ["утверждение", ...]}"""


def _collect_json(text: str) -> dict[str, Any]:
    m = re.search(r"\{.*\}", text, re.S)
    if not m:
        raise ValueError("judge returned no JSON")
    return json.loads(m.group())


async def judge_faithfulness(llm: LLM, req: AgentChatRequest, answer: str, source_ids: list[str],
                             kb: list[Doc]) -> dict[str, Any]:
    docs = [d for d in kb if d.id in set(source_ids)]
    documents = "\n\n".join(f"«{d.title}» (обновлено {d.updated or '—'})\n{d.text}" for d in docs) \
        or "(нет процитированных документов)"
    context = json.dumps(req.user_context.model_dump(mode="json", exclude_none=True), ensure_ascii=False)
    user = f"ВОПРОС:\n{req.message}\n\nОТВЕТ:\n{answer}\n\nДОКУМЕНТЫ:\n{documents}\n\nКОНТЕКСТ:\n{context}"
    turn = Turn(req, [], [{"role": "system", "content": JUDGE_PROMPT}, {"role": "user", "content": user}], [])
    text = "".join([ev.text async for ev in llm.stream(turn) if isinstance(ev, TextDelta)])
    verdict = _collect_json(text)
    return {"supported": bool(verdict.get("supported")), "unsupported": list(verdict.get("unsupported") or [])}
