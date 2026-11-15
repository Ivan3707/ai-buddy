"""Pydantic models mirroring contracts/ai.openapi.json (source: contracts/build_contracts.py)."""
from __future__ import annotations

from datetime import date, datetime
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

ActionType = Literal["leave_request", "access_request", "mentor_question"]


def _nonblank(value: str) -> str:
    if not value.strip():
        raise ValueError("must not be blank")
    return value


class ChatRequest(BaseModel):
    """Deprecated POST /chat."""
    model_config = ConfigDict(extra="forbid")
    message: str = Field(min_length=1, max_length=2000)
    check_nonblank = field_validator("message")(_nonblank)


class Source(BaseModel):
    id: str
    title: str
    url: str
    snippet: str | None = None


class ChatResponse(BaseModel):
    answer: str
    sources: list[Source]
    mode: Literal["mock", "llm"] = "mock"


# ---- action parameters: what the LLM fills in, validated before anything leaves the AI ----

class LeaveRequestParams(BaseModel):
    model_config = ConfigDict(extra="forbid")
    kind: Literal["day_off", "vacation"]
    date_from: date
    date_to: date
    comment: str | None = Field(default=None, max_length=500)

    @model_validator(mode="after")
    def ordered(self) -> LeaveRequestParams:
        if self.date_to < self.date_from:
            raise ValueError("date_to must not be before date_from")
        return self


class AccessRequestParams(BaseModel):
    model_config = ConfigDict(extra="forbid")
    system: Literal["jira", "confluence", "gitlab", "grafana", "redash", "kubernetes"]
    resource: str = Field(min_length=1, max_length=200)
    access_level: Literal["read", "write", "admin"]
    justification: str = Field(min_length=1, max_length=500)
    check_nonblank = field_validator("resource", "justification")(_nonblank)


class MentorQuestionParams(BaseModel):
    model_config = ConfigDict(extra="forbid")
    question: str = Field(min_length=1, max_length=2000)
    context: str | None = Field(default=None, max_length=2000)
    check_nonblank = field_validator("question")(_nonblank)


class ActionProposal(BaseModel):
    type: ActionType
    params: dict[str, Any]
    summary: str


# ---- verified context sent by Backend ----

class TaskContext(BaseModel):
    id: str
    title: str
    description: str | None = None
    status: str
    checklist: Literal["common", "professional"]
    stage_title: str | None = None
    requires_result: bool | None = None
    review_comment: str | None = None


class ServiceRequest(BaseModel):
    id: str
    number: str
    type: Literal["leave_request", "access_request"]
    status: Literal["pending_approval", "approved", "rejected"]
    approver: str | None = None
    params: dict[str, Any]
    created_at: str
    updated_at: str


class Stage(BaseModel):
    title: str
    position: int
    total: int


class UserContext(BaseModel):
    user_id: str
    display_name: str | None = None
    role: Literal["intern", "buddy", "teamlead"]
    profession: Literal["backend", "frontend", "ml"] | None = None
    team: str | None = None
    buddy_name: str | None = None
    today: date | None = None
    current_stage: Stage | None = None
    tasks: list[TaskContext] = Field(max_length=30)
    open_requests: list[ServiceRequest] = Field(max_length=20)
    allowed_actions: list[ActionType]


class ChatTurn(BaseModel):
    role: Literal["user", "assistant"]
    content: str = Field(max_length=8000)


class PageContext(BaseModel):
    """Page open in the browser (extension side panel). Untrusted data, never instructions."""
    model_config = ConfigDict(extra="forbid")
    url: str = Field(max_length=2000)
    title: str | None = Field(default=None, max_length=300)
    selection: str | None = Field(default=None, max_length=2000)


class BriefRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    user_context: UserContext
    last_seen_at: datetime | None = None


class AgentChatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    message: str = Field(min_length=1, max_length=2000)
    history: list[ChatTurn] = Field(default_factory=list, max_length=20)
    user_context: UserContext
    page_context: PageContext | None = None
    check_nonblank = field_validator("message")(_nonblank)
