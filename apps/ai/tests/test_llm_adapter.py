"""OpenAI-compatible adapter against a fake HTTP transport (no network)."""
import asyncio
import json

import httpx
import pytest

from app.config import Settings, load_settings
from app.llm import LLMError, OpenAICompatibleLLM, TextDelta, ToolCall, Turn, Usage
from app.schemas import AgentChatRequest
from conftest import user_context

SETTINGS = Settings(provider="openai_compatible", base_url="https://llm.test/v1", api_key="k", model="m",
                    temperature=0.2)
TURN = Turn(AgentChatRequest(message="hi", user_context=user_context()), [], [{"role": "user", "content": "hi"}],
            [{"type": "function", "function": {"name": "propose_mentor_question"}}])


def sse_body(*chunks) -> bytes:
    lines = [": keep-alive", ""]
    lines += [f"data: {c if isinstance(c, str) else json.dumps(c)}\n" for c in chunks]
    return "\n".join(lines).encode()


def run(handler, settings=SETTINGS):
    llm = OpenAICompatibleLLM(settings, transport=httpx.MockTransport(handler))

    async def collect():
        return [ev async for ev in llm.stream(TURN)]
    return asyncio.run(collect())


def test_streams_text_tool_call_fragments_and_usage():
    seen = {}

    def handler(request: httpx.Request):
        seen["url"] = str(request.url)
        seen["auth"] = request.headers["authorization"]
        seen["body"] = json.loads(request.content)
        return httpx.Response(200, content=sse_body(
            {"choices": [{"delta": {"content": "При"}}]},
            {"choices": [{"delta": {"content": "вет"}}]},
            {"choices": [{"delta": {"tool_calls": [{"index": 0, "function": {"name": "propose_mentor_question",
                                                                              "arguments": '{"quest'}}]}}]},
            {"choices": [{"delta": {"tool_calls": [{"index": 0, "function": {"arguments": 'ion":"Q"}'}}]}}]},
            {"choices": [], "usage": {"prompt_tokens": 10, "completion_tokens": 3}},
            "[DONE]"))

    events = run(handler)
    assert events == [TextDelta("При"), TextDelta("вет"), Usage(10, 3),
                      ToolCall("propose_mentor_question", '{"question":"Q"}')]
    assert seen["url"] == "https://llm.test/v1/chat/completions" and seen["auth"] == "Bearer k"
    body = seen["body"]
    assert body["stream"] and body["model"] == "m" and body["temperature"] == 0.2 and body["tools"]


@pytest.mark.parametrize("handler,code", [
    (lambda r: httpx.Response(401, content=b"bad key"), "llm_unavailable"),
    (lambda r: httpx.Response(200, content=sse_body({"error": {"message": "x"}})), "llm_unavailable"),
    (lambda r: httpx.Response(200, content=sse_body("{not json")), "llm_unavailable"),
])
def test_provider_failures_map_to_error_codes(handler, code):
    with pytest.raises(LLMError) as e:
        run(handler)
    assert e.value.code == code


def test_network_errors_and_timeouts():
    def refuse(request):
        raise httpx.ConnectError("refused")

    def slow(request):
        raise httpx.ReadTimeout("slow")

    with pytest.raises(LLMError) as e:
        run(refuse)
    assert e.value.code == "llm_unavailable"
    with pytest.raises(LLMError) as e:
        run(slow)
    assert e.value.code == "timeout"


def test_total_deadline():
    settings = Settings(provider="openai_compatible", base_url="https://llm.test/v1", api_key="k", model="m",
                        timeout_s=-1)
    with pytest.raises(LLMError) as e:
        run(lambda r: httpx.Response(200, content=sse_body({"choices": []})), settings)
    assert e.value.code == "timeout"


def test_load_settings():
    assert load_settings({}).mode == "mock"
    s = load_settings({"LLM_PROVIDER": "openai_compatible", "LLM_BASE_URL": "https://x/v1/", "LLM_API_KEY": "k",
                       "LLM_MODEL": "m", "LLM_TEMPERATURE": "0.3", "RAG_TOP_K": "2"})
    assert (s.mode, s.base_url, s.temperature, s.top_k) == ("llm", "https://x/v1", 0.3, 2)
    with pytest.raises(ValueError, match="LLM_API_KEY"):
        load_settings({"LLM_PROVIDER": "openai_compatible", "LLM_BASE_URL": "u", "LLM_MODEL": "m"})
    with pytest.raises(ValueError, match="LLM_PROVIDER"):
        load_settings({"LLM_PROVIDER": "gpt"})
