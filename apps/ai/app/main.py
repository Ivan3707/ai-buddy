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

@app.get("/health")
def health():
    return {"status": "ok", "mode": "mock"}

@app.post("/chat", response_model=ChatResponse)
def chat(body: ChatRequest):
    query = body.message.casefold()
    doc = next((item for item in KB if any(word in query for word in item["keywords"])), None)
    if doc is None:
        return ChatResponse(answer="В демо-базе нет ответа. Попробуйте спросить про отпуск или доступ к сервисам.", sources=[])
    return ChatResponse(answer=doc["text"], sources=[Source(**{k: doc[k] for k in ("id", "title", "url")})])
