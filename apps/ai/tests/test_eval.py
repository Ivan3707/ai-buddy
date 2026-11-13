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
    """Gate for retriever changes: BM25 baseline is 0.94 recall@4 on 2026-11-13."""
    res = eval_retrieval(load_cases(), k=4)
    assert res["recall"] >= 0.9, res["misses"]


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
