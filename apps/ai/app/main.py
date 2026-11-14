"""Deterministic retrieval placeholder; no external LLM or real integrations."""
import json
import os
from pathlib import Path
from fastapi import FastAPI
from pydantic import BaseModel, Field, ConfigDict, field_validator

app = FastAPI(title="AI-buddy AI", version="0.1.0")
DATA = Path(os.getenv("DATA_DIR") or (Path(__file__).resolve().parents[3] / "data"))
KB = json.loads((DATA / "kb.json").read_text(encoding="utf-8"))

class ChatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    message: str = Field(min_length=1, max_length=2000)

    @field_validator("message")
    @classmethod
    def nonblank(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("message must not be blank")
        return value

class Source(BaseModel):
    id: str
    title: str
    url: str

class ChatResponse(BaseModel):
    answer: str
    sources: list[Source]
    mode: str = "mock"

class HintRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    title: str = Field(min_length=1, max_length=255)
    description: str = Field(default="", max_length=4000)
    expected_result: str = Field(default="", max_length=4000)
    estimated_minutes: int = Field(gt=0)

class HintResponse(BaseModel):
    hint: str

def find_document(text: str):
    query = text.casefold()
    return next((item for item in KB if any(word in query for word in item["keywords"])), None)

@app.get("/health")
def health():
    return {"status": "ok", "mode": "mock"}

@app.post("/chat", response_model=ChatResponse)
def chat(body: ChatRequest):
    doc = find_document(body.message)
    if doc is None:
        return ChatResponse(answer="В демо-базе нет ответа. Попробуйте спросить про отпуск или доступ к сервисам.", sources=[])
    return ChatResponse(answer=doc["text"], sources=[Source(**{k: doc[k] for k in ("id", "title", "url")})])

@app.post("/hint", response_model=HintResponse)
def hint(body: HintRequest):
    doc = find_document(" ".join((body.title, body.description, body.expected_result)))
    if doc is not None:
        return HintResponse(hint=doc["text"])
    expected = body.expected_result.strip()
    guidance = f"Начните с небольшого шага по задаче «{body.title.strip()}»."
    if expected:
        guidance += f" Сверьте результат с ожидаемым: {expected}."
    return HintResponse(hint=guidance)
