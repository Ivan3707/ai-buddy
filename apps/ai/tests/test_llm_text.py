import asyncio

from app.llm import MockLLM, TextDelta, Usage
from app.llm_text import complete, is_mock, parse_json_object


class Echo:
    def __init__(self):
        self.turns = []

    async def stream(self, turn):
        self.turns.append(turn)
        yield TextDelta(" при")
        yield TextDelta("вет ")
        yield Usage(3, 2)


def test_complete_collects_text_and_usage():
    llm = Echo()
    text, usage = asyncio.run(complete(llm, [{"role": "system", "content": "s"}, {"role": "user", "content": "u"}]))
    assert text == "привет" and usage == Usage(3, 2)
    turn = llm.turns[0]
    assert turn.messages[-1]["content"] == "u" and turn.tools == [] and turn.hits == []
    assert is_mock(MockLLM()) and not is_mock(llm)


def test_parse_json_object():
    assert parse_json_object('```json\n{"a": 1}\n```') == {"a": 1}
    assert parse_json_object("ответ: {\"a\": [1, 2]} — всё") == {"a": [1, 2]}
    assert parse_json_object("нет json") is None
    assert parse_json_object("{битый}") is None
    assert parse_json_object("[{}]") == {}
    assert parse_json_object('{"a": 1} и {"b": 2}') is None  # greedy span is not valid JSON
