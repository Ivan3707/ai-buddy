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
            {"choices": [], "usage": {"prompt_tokens": 10, "completion_tokens": 3, "prompt_tokens_details": {"cached_tokens": 8}}},
            "[DONE]"))

    events = run(handler)
    assert events == [TextDelta("При"), TextDelta("вет"), Usage(10, 3, 8),
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


def test_fallback_llm_only_before_first_output():
    from app.llm import FallbackLLM, make_llm

    class Scripted:
        def __init__(self, *events, error=None):
            self.events, self.error, self.calls = events, error, 0

        async def stream(self, turn):
            self.calls += 1
            for ev in self.events:
                yield ev
            if self.error:
                raise self.error

    def run_llm(llm):
        async def collect():
            return [ev async for ev in llm.stream(TURN)]
        return asyncio.run(collect())

    backup = Scripted(TextDelta("локально"))
    assert run_llm(FallbackLLM(Scripted(Usage(1, 0), error=LLMError("llm_unavailable")), backup, "m")) == \
        [Usage(1, 0), TextDelta("локально")]
    with pytest.raises(LLMError):  # already answered something: do not glue two answers
        run_llm(FallbackLLM(Scripted(TextDelta("Нача"), error=LLMError("timeout")), Scripted(), "m"))
    unused = Scripted()
    assert run_llm(FallbackLLM(Scripted(TextDelta("ok")), unused, "m")) == [TextDelta("ok")] and unused.calls == 0

    env = {"LLM_PROVIDER": "openai_compatible", "LLM_BASE_URL": "https://x/v1", "LLM_API_KEY": "k", "LLM_MODEL": "m",
           "LLM_FALLBACK_BASE_URL": "http://ollama:11434/v1/", "LLM_FALLBACK_MODEL": "qwen3.5:4b",
           "LLM_FALLBACK_EXTRA_BODY": '{"reasoning_effort": "none"}'}
    llm = make_llm(load_settings(env))
    assert isinstance(llm, FallbackLLM) and llm.fallback.s.base_url == "http://ollama:11434/v1"
    assert llm.fallback.s.api_key == "none" and llm.fallback.s.extra_body == {"reasoning_effort": "none"}
    assert isinstance(make_llm(load_settings({k: v for k, v in env.items() if "FALLBACK" not in k})), OpenAICompatibleLLM)
