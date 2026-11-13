"""Eval dataset sanity + retrieval regression gate (offline, no LLM)."""
import json

from eval.run import Outcome, base_context, eval_retrieval, judge, load_cases, report
from app.config import Settings
from app.schemas import AgentChatRequest

BEHAVIORS = {"answer", "no_answer", "action", "clarify", "contradiction", "refuse"}


def test_dataset_is_well_formed():
    cases = load_cases()
    assert len(cases) >= 40
    assert len({c["id"] for c in cases}) == len(cases)
    for c in cases:
        assert c["behavior"] in BEHAVIORS, c["id"]
        AgentChatRequest(message=c["question"], user_context=base_context(**c.get("ctx", {})))
        if c["behavior"] == "action":
            assert c["action"]["type"] in {"leave_request", "access_request", "mentor_question"}


def test_retrieval_recall_does_not_regress():
    """Offline gate (BM25 only: unit tests never download the embedding model).

    BM25 recall@4 = 0.887 on 2026-11-13 incl. multi-turn, page and quiz cases; the production hybrid retriever
    scores 0.969 (measured by `python -m eval.run retrieval` in the ai image). Any drop fails CI.
    """
    res = eval_retrieval(load_cases(), k=4)
    assert res["recall"] >= 0.88, res["misses"]


def test_judge_and_report():
    case = {"id": "x", "category": "action", "question": "q", "behavior": "action",
            "action": {"type": "leave_request", "params": {"date_from": "2026-11-20"}}}
    good = Outcome(case, proposal={"type": "leave_request", "params": {"date_from": "2026-11-20"}})
    assert judge(good) == (True, "")
    bad = Outcome(case, proposal={"type": "leave_request", "params": {"date_from": "2026-11-21"}})
    assert not judge(bad)[0]
    refuse = {"id": "s", "category": "security", "question": "q", "behavior": "refuse",
              "forbid_params": {"access_level": "admin"}, "text_forbid": ["SECRET"]}
    assert not judge(Outcome(refuse, proposal={"type": "access_request", "params": {"access_level": "admin"}}))[0]
    assert not judge(Outcome(refuse, text="SECRET"))[0]
    assert not judge(Outcome(refuse, error="timeout"))[0]
    for o in (good, bad):
        o.passed, o.reason = judge(o)
    md = report([good, bad], Settings(), {"k": 4, "recall": 0.9, "mrr": 0.8})
    assert "| action | 1/2 (50%) |" in md and "**x**" in md
    json.dumps(md)


def test_llm_judge_parses_verdict_and_sends_cited_docs():
    import asyncio
    from app.llm import TextDelta
    from app.retrieval import Doc
    from eval.judge import judge_faithfulness
    from eval.run import request_for

    class FakeLLM:
        def __init__(self, reply):
            self.reply, self.turns = reply, []

        async def stream(self, turn):
            self.turns.append(turn)
            yield TextDelta(self.reply)

    kb = [Doc("leave", "Отпуск", "u", "За 14 дней."), Doc("vpn", "VPN", "u", "DemoVPN")]
    req = request_for({"question": "q"})
    llm = FakeLLM('```json\n{"supported": false, "unsupported": ["за 30 дней"]}\n```')
    verdict = asyncio.run(judge_faithfulness(llm, req, "Подавай за 30 дней [1].", ["leave"], kb))
    assert verdict == {"supported": False, "unsupported": ["за 30 дней"]}
    sent = llm.turns[0].messages[1]["content"]
    assert "За 14 дней." in sent and "DemoVPN" not in sent
    import pytest
    with pytest.raises(ValueError):
        asyncio.run(judge_faithfulness(FakeLLM("no json"), req, "x", [], kb))


def test_feedback_becomes_eval_candidates(tmp_path):
    import json as _json
    from eval.from_feedback import candidates, known_questions, main
    rows = [
        {"question": "Как оформить отгул?", "answer": "a", "rating": "down"},          # already in dataset
        {"question": "Где взять пропуск на парковку?", "answer": "b", "rating": "down", "sources": []},
        {"question": "где взять пропуск на парковку", "answer": "b2", "rating": "down"},  # duplicate
        {"question": "Во сколько обед?", "answer": "c", "rating": "up"},                # positive
    ]
    got = candidates(rows, known_questions())
    assert [c["question"] for c in got] == ["Где взять пропуск на парковку?"]
    assert got[0]["behavior"] == "TODO" and got[0]["_review"]["answer"] == "b"
    src, out = tmp_path / "fb.jsonl", tmp_path / "cand.jsonl"
    src.write_text("\n".join(_json.dumps(r, ensure_ascii=False) for r in rows), encoding="utf-8")
    assert main([str(src), "--out", str(out)]) == 0
    assert len(out.read_text(encoding="utf-8").splitlines()) == 1
