"""Generate contracts/*.openapi.json from one source of truth.

Shared schemas (chat, actions, service requests) are defined once here so the
public Backend contract and the internal AI contract cannot drift apart.
Run from repo root:  python contracts/build_contracts.py
"""
import json
from copy import deepcopy
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def ref(name):
    return {"$ref": f"#/components/schemas/{name}"}


def json_body(schema):
    return {"required": True, "content": {"application/json": {"schema": schema}}}


def json_resp(description, schema):
    return {"description": description, "content": {"application/json": {"schema": schema}}}


ERROR = {"type": "object", "required": ["error"], "properties": {"error": {"type": "string"}}}
MESSAGE = {"type": "string", "minLength": 1, "maxLength": 2000, "pattern": "\\S"}
SSE_DESCRIPTION = (
    "Server-Sent Events stream. Each event is `event: <type>` + `data: <JSON>` "
    "where JSON matches the schema of the same `type`. Order: [meta] status* "
    "(delta | status)* [sources] [action] done | error. The stream always ends "
    "with exactly one `done` or `error` event."
)

# ---------------------------------------------------------------- shared
SHARED = {
    # legacy non-streaming chat, kept until web/extension migrate to /stream
    "ChatRequest": {
        "type": "object", "additionalProperties": False, "required": ["message"],
        "properties": {"message": MESSAGE},
    },
    "ChatResponse": {
        "type": "object", "required": ["answer", "sources", "mode"],
        "properties": {
            "answer": {"type": "string"},
            "sources": {"type": "array", "items": ref("Source")},
            "mode": ref("AiMode"),
        },
    },
    "AiMode": {"type": "string", "enum": ["mock", "llm"],
               "description": "mock = deterministic offline mode (tests/CI/no key); llm = real model"},
    "Source": {
        "type": "object", "required": ["id", "title", "url"],
        "properties": {
            "id": {"type": "string"}, "title": {"type": "string"}, "url": {"type": "string"},
            "snippet": {"type": "string", "description": "Short quote used for the answer"},
        },
    },
    "ActionType": {"type": "string", "enum": ["leave_request", "access_request", "mentor_question"]},
    "LeaveRequestParams": {
        "type": "object", "additionalProperties": False,
        "required": ["kind", "date_from", "date_to"],
        "properties": {
            "kind": {"type": "string", "enum": ["day_off", "vacation"],
                     "description": "Отгул и отпуск — разные процессы"},
            "date_from": {"type": "string", "format": "date"},
            "date_to": {"type": "string", "format": "date"},
            "comment": {"type": "string", "maxLength": 500},
        },
    },
    "AccessRequestParams": {
        "type": "object", "additionalProperties": False,
        "required": ["system", "resource", "access_level", "justification"],
        "properties": {
            "system": {"type": "string", "enum": ["jira", "confluence", "gitlab", "grafana", "redash", "kubernetes"]},
            "resource": {"type": "string", "maxLength": 200,
                         "description": "Проект Jira, пространство Confluence, репозиторий, дашборд и т.п."},
            "access_level": {"type": "string", "enum": ["read", "write", "admin"]},
            "justification": {"type": "string", "minLength": 1, "maxLength": 500},
        },
    },
    "MentorQuestionParams": {
        "type": "object", "additionalProperties": False, "required": ["question"],
        "properties": {
            "question": {"type": "string", "minLength": 1, "maxLength": 2000},
            "context": {"type": "string", "maxLength": 2000,
                        "description": "Что бот уже нашёл/почему не смог ответить (противоречие, нет документа)"},
        },
    },
    "ActionProposal": {
        "type": "object", "required": ["type", "params", "summary"],
        "description": "Proposed by the AI. Never executed by the AI: Backend stores it as PendingAction "
                       "and executes only after explicit user confirmation.",
        "properties": {
            "type": ref("ActionType"),
            "params": {"oneOf": [ref("LeaveRequestParams"), ref("AccessRequestParams"), ref("MentorQuestionParams")]},
            "summary": {"type": "string", "description": "Human-readable card text, e.g. 'Отгул 20.11.2026'"},
        },
    },
    "ServiceRequest": {
        "type": "object",
        "required": ["id", "number", "type", "status", "params", "created_at", "updated_at"],
        "description": "A request created in a (mocked) corporate service: absence or access system.",
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "number": {"type": "string", "examples": ["ABS-000123", "ACC-000045"]},
            "type": {"type": "string", "enum": ["leave_request", "access_request"]},
            "status": {"type": "string", "enum": ["pending_approval", "approved", "rejected"]},
            "approver": {"type": "string", "description": "Роль/имя согласующего (руководитель, владелец проекта)"},
            "params": {"oneOf": [ref("LeaveRequestParams"), ref("AccessRequestParams")]},
            "created_at": {"type": "string", "format": "date-time"},
            "updated_at": {"type": "string", "format": "date-time"},
        },
    },
    "TaskContext": {
        "type": "object", "required": ["id", "title", "status", "checklist"],
        "properties": {
            "id": {"type": "string"}, "title": {"type": "string"},
            "description": {"type": "string"},
            "status": {"type": "string", "description": "As stored by Backend (unavailable/active/submitted/completed/...)"},
            "checklist": {"type": "string", "enum": ["common", "professional"]},
            "stage_title": {"type": "string"},
            "requires_result": {"type": "boolean"},
            "review_comment": {"type": "string", "description": "Последний комментарий наставника, если есть"},
        },
    },
    "UserContext": {
        "type": "object", "required": ["user_id", "role", "tasks", "open_requests", "allowed_actions"],
        "description": "Verified by Backend from the JWT and DB. The AI trusts only this, never claims from chat text.",
        "properties": {
            "user_id": {"type": "string"},
            "display_name": {"type": "string"},
            "role": {"type": "string", "enum": ["intern", "buddy"]},
            "profession": {"type": ["string", "null"], "enum": ["backend", "frontend", "ml", None]},
            "team": {"type": "string"},
            "buddy_name": {"type": "string"},
            "today": {"type": "string", "format": "date", "description": "Для разбора 'завтра', 'в пятницу'"},
            "current_stage": {
                "type": "object", "required": ["title", "position", "total"],
                "properties": {"title": {"type": "string"}, "position": {"type": "integer"},
                               "total": {"type": "integer"}},
            },
            "tasks": {"type": "array", "maxItems": 30, "items": ref("TaskContext"),
                      "description": "Active and next tasks first"},
            "open_requests": {"type": "array", "maxItems": 20, "items": ref("ServiceRequest")},
            "allowed_actions": {"type": "array", "items": ref("ActionType"),
                                "description": "RBAC: actions this user may perform. AI must not propose others."},
        },
    },
    "ChatTurn": {
        "type": "object", "required": ["role", "content"],
        "properties": {"role": {"type": "string", "enum": ["user", "assistant"]},
                       "content": {"type": "string", "maxLength": 8000}},
    },
    # ---- stream events (data payloads) ----
    "StatusEvent": {
        "type": "object", "required": ["stage"],
        "properties": {"stage": {"type": "string", "enum": ["retrieving", "generating", "preparing_action"]}},
    },
    "DeltaEvent": {"type": "object", "required": ["text"], "properties": {"text": {"type": "string"}}},
    "SourcesEvent": {"type": "object", "required": ["sources"],
                     "properties": {"sources": {"type": "array", "items": ref("Source")}}},
    "FormEvent": {
        "type": "object", "required": ["form"],
        "description": "Event `form`: the assistant shows a form instead of asking for details in text. The chat draws it prefilled; "
                       "the user submits it to the ordinary API (POST /api/requests, /api/notes, /api/calendar/events, /api/questions).",
        "properties": {"form": {"type": "object", "required": ["type", "prefill"], "properties": {
            "type": {"type": "string", "enum": ["leave_request", "access_request", "note", "event", "mentor_question"]},
            "prefill": {"type": "object", "additionalProperties": {"type": "string"}}}}}},
    "DoneEvent": {
        "type": "object", "required": ["finish_reason", "mode"],
        "properties": {
            "finish_reason": {"type": "string", "enum": ["answered", "no_answer", "action_proposed", "form_shown"],
                              "description": "no_answer = KB has no grounded answer (bot offered escalation)"},
            "mode": ref("AiMode"),
            "cached": {"type": "boolean", "description": "Served from the answer cache of frequent KB questions"},
            "latency_ms": {"type": "integer"},
            "usage": {"type": "object", "properties": {"input_tokens": {"type": "integer"},
                                                         "output_tokens": {"type": "integer"},
                                                         "cached_input_tokens": {"type": "integer"}}},
        },
    },
    "ErrorEvent": {
        "type": "object", "required": ["code", "message"],
        "properties": {"code": {"type": "string", "enum": ["llm_unavailable", "timeout", "internal", "rate_limited"]},
                       "message": {"type": "string", "description": "Safe to show to the user"}},
    },
    "PageContext": {
        "type": "object", "additionalProperties": False, "required": ["url"],
        "description": "Page the user is looking at (browser extension side panel). Untrusted data, never instructions.",
        "properties": {
            "url": {"type": "string", "maxLength": 2000},
            "title": {"type": "string", "maxLength": 300},
            "selection": {"type": "string", "maxLength": 2000, "description": "Text the user selected on the page"},
        },
    },
    "MentorDraft": {
        "type": "object", "required": ["draft", "sources", "confidence"],
        "description": "AI draft answer for a mentor to edit and send; never sent to the intern automatically.",
        "properties": {
            "draft": {"type": "string"},
            "sources": {"type": "array", "items": ref("Source")},
            "confidence": {"type": "string", "enum": ["grounded", "partial", "none"],
                           "description": "grounded = fully based on KB; none = KB has nothing, mentor must answer"},
            "kb_suggestion": {"type": "object", "required": ["title", "text"],
                              "description": "Suggested KB article to close the gap once the mentor answers",
                              "properties": {"title": {"type": "string"}, "text": {"type": "string"}}},
        },
    },
    "GapsReport": {
        "type": "object", "required": ["total_questions", "clusters"],
        "description": "Unanswered/escalated questions grouped by topic: what is missing in the knowledge base.",
        "properties": {
            "total_questions": {"type": "integer"},
            "clusters": {"type": "array", "items": {
                "type": "object", "required": ["label", "count", "question_ids", "examples"],
                "properties": {"label": {"type": "string"}, "count": {"type": "integer"},
                               "question_ids": {"type": "array", "items": {"type": "string"}},
                               "examples": {"type": "array", "maxItems": 3, "items": {"type": "string"}}}}},
        },
    },
    "KbDocument": {
        "type": "object", "required": ["id", "title", "url", "updated", "sections"],
        "description": "One knowledge-base document with its sections in order (reader in the chat).",
        "properties": {
            "id": {"type": "string"}, "title": {"type": "string"},
            "url": {"type": "string", "description": "Full page in the (demo) wiki"},
            "updated": {"type": "string", "description": "YYYY-MM-DD or empty"},
            "sections": {"type": "array", "items": {"type": "object", "required": ["heading", "text"], "properties": {
                "heading": {"type": "string"}, "text": {"type": "string", "description": "Markdown"}}}}}},
    "KbLintReport": {
        "type": "object", "required": ["generated_at", "issues"],
        "description": "Knowledge-base hygiene: contradictions between documents and stale documents.",
        "properties": {
            "generated_at": {"type": "string", "format": "date-time"},
            "issues": {"type": "array", "items": {
                "type": "object", "required": ["kind", "doc_ids", "detail"],
                "properties": {"kind": {"type": "string", "enum": ["contradiction", "stale"]},
                               "doc_ids": {"type": "array", "items": {"type": "string"}},
                               "detail": {"type": "string"},
                               "sources": {"type": "array", "items": ref("Source")}}}},
        },
    },
}

# ---------------------------------------------------------------- AI-only
AI_ONLY = {
    "AgentChatRequest": {
        "type": "object", "additionalProperties": False, "required": ["message", "user_context"],
        "properties": {
            "message": MESSAGE,
            "history": {"type": "array", "maxItems": 20, "items": ref("ChatTurn"),
                        "description": "Previous turns, oldest first, without the current message"},
            "user_context": ref("UserContext"),
            "page_context": ref("PageContext"),
        },
    },
    "AiActionEvent": {"type": "object", "required": ["proposal"], "properties": {"proposal": ref("ActionProposal")}},
    "BriefRequest": {
        "type": "object", "additionalProperties": False, "required": ["user_context"],
        "properties": {"user_context": ref("UserContext"),
                       "last_seen_at": {"type": "string", "format": "date-time",
                                        "description": "When the intern last opened the chat: brief mentions changes since"}},
    },
    "MentorDraftRequest": {
        "type": "object", "additionalProperties": False, "required": ["question"],
        "properties": {"question": {"type": "string", "minLength": 1, "maxLength": 2000},
                       "context": {"type": "string", "maxLength": 2000},
                       "user_context": ref("UserContext")},
    },
    "GapsRequest": {
        "type": "object", "additionalProperties": False, "required": ["questions"],
        "properties": {"questions": {"type": "array", "maxItems": 2000, "items": {
            "type": "object", "required": ["id", "text"],
            "properties": {"id": {"type": "string"}, "text": {"type": "string", "maxLength": 2000}}}}},
    },
}

# ---------------------------------------------------------------- Backend-only
BACKEND_ONLY = {
    "StreamChatRequest": {
        "type": "object", "additionalProperties": False, "required": ["message"],
        "properties": {
            "message": MESSAGE,
            "conversation_id": {"type": "string", "format": "uuid",
                                "description": "Omit to start a new conversation"},
            "page_context": ref("PageContext"),
        },
    },
    "MetaEvent": {
        "type": "object", "required": ["conversation_id", "message_id"],
        "properties": {"conversation_id": {"type": "string", "format": "uuid"},
                       "message_id": {"type": "string", "format": "uuid",
                                      "description": "Id of the assistant message being streamed"}},
    },
    "PendingAction": {
        "type": "object", "required": ["id", "type", "params", "summary", "status", "created_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "type": ref("ActionType"),
            "params": {"oneOf": [ref("LeaveRequestParams"), ref("AccessRequestParams"), ref("MentorQuestionParams")]},
            "summary": {"type": "string"},
            "status": {"type": "string", "enum": ["pending", "executed", "cancelled", "failed"]},
            "error": {"type": "string"},
            "created_at": {"type": "string", "format": "date-time"},
        },
    },
    "BackendActionEvent": {"type": "object", "required": ["action"], "properties": {"action": ref("PendingAction")}},
    "ActionResult": {
        "type": "object", "required": ["action"],
        "properties": {
            "action": ref("PendingAction"),
            "request": ref("ServiceRequest"),
            "mentor_question": ref("MentorQuestion"),
        },
    },
    "MentorQuestion": {
        "type": "object", "required": ["id", "intern_id", "question", "status", "created_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "intern_id": {"type": "string", "format": "uuid"},
            "intern_name": {"type": "string"},
            "question": {"type": "string"}, "context": {"type": "string"},
            "status": {"type": "string", "enum": ["open", "answered"]},
            "escalation_level": {"type": "string", "enum": ["mentor", "lead", "hrbp"],
                                 "description": "Raised mentor -> team lead -> HRBP when unanswered past the SLA"},
            "escalated_at": {"type": "string", "format": "date-time"},
            "answer": {"type": "string"},
            "created_at": {"type": "string", "format": "date-time"},
            "answered_at": {"type": "string", "format": "date-time"},
        },
    },
    "ChatMessage": {
        "type": "object", "required": ["id", "role", "content", "created_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "role": {"type": "string", "enum": ["user", "assistant"]},
            "content": {"type": "string"},
            "sources": {"type": "array", "items": ref("Source")},
            "action": ref("PendingAction"),
            "finish_reason": {"type": "string"},
            "feedback": {"type": ["string", "null"], "enum": ["up", "down", None]},
            "created_at": {"type": "string", "format": "date-time"},
        },
    },
    "Feedback": {"type": "object", "additionalProperties": False, "required": ["rating"],
                 "properties": {"rating": {"type": "string", "enum": ["up", "down"]},
                                "comment": {"type": "string", "maxLength": 1000,
                                            "description": "What was wrong; only kept with rating=down. Shown to the admin, not to mentors."}}},
    "AnswerMentorQuestion": {"type": "object", "additionalProperties": False, "required": ["answer"],
                             "properties": {"answer": {"type": "string", "minLength": 1, "maxLength": 4000}}},
    "ChatMetrics": {
        "type": "object", "description": "Product metrics for the demo dashboard / presentation",
        "properties": {
            "questions_total": {"type": "integer"},
            "answered_by_bot": {"type": "integer"},
            "escalated_to_mentor": {"type": "integer"},
            "deflection_rate": {"type": "number", "description": "1 - escalated/questions"},
            "actions_executed": {"type": "integer"},
            "estimated_mentor_minutes_saved": {"type": "number"},
        },
    },
}


def stream_events(action_schema, extra=()):
    names = ["StatusEvent", "DeltaEvent", "SourcesEvent", action_schema, "FormEvent", "DoneEvent", "ErrorEvent", *extra]
    return {"oneOf": [ref(n) for n in names]}


def sse(description):
    return {"description": description,
            "content": {"text/event-stream": {"schema": {"type": "string",
                                                          "description": SSE_DESCRIPTION}}}}


BEARER = [{"bearerAuth": []}]
UUID_PARAM = {"name": "id", "in": "path", "required": True, "schema": {"type": "string", "format": "uuid"}}

# Admin panel (teamlead only on the backend; AI endpoints are internal)
INT = {"type": "integer"}
STR = {"type": "string"}
ADMIN_BALANCE = {"type": "object", "required": ["available", "note"], "properties": {
    "available": {"type": "boolean"}, "currency": STR, "usage": {"type": ["number", "null"]},
    "limit": {"type": ["number", "null"]}, "limit_remaining": {"type": ["number", "null"]},
    "usage_daily": {"type": ["number", "null"]}, "usage_weekly": {"type": ["number", "null"]},
    "usage_monthly": {"type": ["number", "null"]}, "is_free_tier": {"type": ["boolean", "null"]},
    "label": {"type": ["string", "null"]}, "note": {**STR, "description": "Human note when no balance is available"}}}
ADMIN_LLM = {"type": "object", "required": ["provider", "mode", "model", "balance"], "properties": {
    "provider": STR, "mode": ref("AiMode"), "model": STR, "fallback_model": STR, "balance": ADMIN_BALANCE,
    "budget": {"type": ["object", "null"], "properties": {
        "daily_tokens_limit": INT, "daily_tokens_used": INT, "per_minute_limit": INT}}}}
ADMIN_PING = {"type": "object", "required": ["ok", "latency_ms", "answer"], "properties": {
    "ok": {"type": "boolean"}, "mode": ref("AiMode"), "model": STR, "latency_ms": INT, "answer": STR,
    "input_tokens": INT, "output_tokens": INT, "error": STR}}
ADMIN_KB = {"type": "object", "required": ["documents", "chunks"], "properties": {
    "documents": INT, "chunks": INT, "source": STR}}
ADMIN_ACCOUNT = {"type": "object", "required": ["id", "email", "role", "created_at", "active_sessions"], "properties": {
    "id": {"type": "string", "format": "uuid"}, "email": STR, "first_name": STR, "last_name": STR,
    "role": {"type": "string", "enum": ["intern", "buddy", "teamlead"]}, "profession": STR, "team": STR,
    "department": STR, "buddy_name": STR, "teamlead_name": STR,
    "created_at": {"type": "string", "format": "date-time"},
    "last_login_at": {"type": ["string", "null"], "format": "date-time"},
    "active_sessions": {**INT, "description": "Not revoked and not expired sessions"}}}
ADMIN_CHECK = {"type": "object", "required": ["id", "title", "status", "detail", "latency_ms"], "properties": {
    "id": STR, "title": STR, "status": {"type": "string", "enum": ["ok", "error"]}, "detail": STR, "latency_ms": INT}}
ADMIN_SYSTEM = {"type": "object", "required": ["checks", "counters", "hints"], "properties": {
    "checks": {"type": "array", "items": ADMIN_CHECK},
    "counters": {"type": "object", "properties": {
        "users_by_role": {"type": "object", "additionalProperties": INT}, "open_questions": INT,
        "pending_requests": INT, "messages_today": INT, "active_sessions": INT}},
    "hints": {"type": "object", "additionalProperties": INT,
              "description": "task_hints by status (pending, processing, ready) plus unread"}}}
ADMIN_SAMPLE = {"type": "object", "required": ["text"], "properties": {
    "trigger": STR, "text": STR, "detail": STR, "ask": STR}}
ADMIN_NOTIFICATION = {"type": "object", "required": ["id", "title", "source", "when", "cadence", "check", "samples", "live"], "properties": {
    "id": {"type": "string", "enum": ["greeting", "page_hint", "selection", "request_status", "stuck_task",
                                      "mentor_answered", "staff_counts"]},
    "title": STR, "source": {"type": "string", "enum": ["widget", "site", "worker"]}, "when": STR, "cadence": STR,
    "button": STR, "check": {"type": "string", "enum": ["preview", "backend"],
                             "description": "preview: the web shows the sample in the admin's own Бади; backend: POST .../check"},
    "samples": {"type": "array", "items": ADMIN_SAMPLE},
    "live": {"type": "array", "items": {"type": "object", "required": ["label", "value"],
                                         "properties": {"label": STR, "value": INT}}}}}
ADMIN_STUCK_CHECK = {"type": "object", "required": ["overdue_tasks", "pending_before", "ready_before", "pending_after",
                                                    "ready_after", "tasks"], "properties": {
    "overdue_tasks": INT, "pending_before": INT, "ready_before": INT, "pending_after": INT, "ready_after": INT,
    "error": STR, "tasks": {"type": "array", "maxItems": 10, "items": {"type": "object", "properties": {
        "task": STR, "user_email": STR, "hint_status": {"type": "string", "enum": ["none", "pending", "processing", "ready"]},
        "hint_text": {**STR, "maxLength": 200}}}}}}
ADMIN_FORBIDDEN = json_resp("Teamlead only", ERROR)

# ---------------------------------------------------------------- AI spec
ai_spec = {
    "openapi": "3.1.0",
    "info": {"title": "AI-buddy ai (internal)", "version": "0.2.0",
             "description": "Internal API. Called only by Backend inside the Docker network."},
    "servers": [{"url": "http://127.0.0.1:8001"}],
    "paths": {
        "/health": {"get": {"operationId": "health", "responses": {"200": json_resp("Success", {
            "type": "object", "required": ["status", "mode"],
            "properties": {"status": {"type": "string", "enum": ["ok"]}, "mode": ref("AiMode"),
                           "model": {"type": "string"}, "fallback_model": {"type": "string"}}})}}},
        "/chat": {"post": {
            "operationId": "chat", "deprecated": True,
            "description": "Legacy non-streaming chat without user context. Use /v1/chat/stream.",
            "requestBody": json_body(ref("ChatRequest")),
            "responses": {"200": json_resp("Success", ref("ChatResponse")),
                          "422": json_resp("Validation error", {"type": "object"})}}},
        "/v1/chat/stream": {"post": {
            "operationId": "chatStream",
            "description": "Agent turn: retrieval + LLM + optional action proposal. Stateless: Backend sends "
                           "history and verified user context on each call. Event data schemas: "
                           "#/components/schemas/AiStreamEvent (event `action` carries AiActionEvent).",
            "requestBody": json_body(ref("AgentChatRequest")),
            "responses": {"200": sse("Event stream"),
                          "422": json_resp("Validation error", {"type": "object"})}}},
        "/v1/brief/stream": {"post": {
            "operationId": "briefStream",
            "description": "Proactive greeting when the intern opens the chat: stage, active task with a practical "
                           "next step, request status changes, review comments. Same events as chat.",
            "requestBody": json_body(ref("BriefRequest")),
            "responses": {"200": sse("Event stream"), "422": json_resp("Validation error", {"type": "object"})}}},
        "/v1/mentor/draft": {"post": {
            "operationId": "mentorDraft",
            "description": "Draft answer for an escalated question, for the mentor to edit.",
            "requestBody": json_body(ref("MentorDraftRequest")),
            "responses": {"200": json_resp("Draft", ref("MentorDraft")),
                          "502": json_resp("LLM unavailable", ERROR)}}},
        "/v1/analytics/gaps": {"post": {
            "operationId": "knowledgeGaps",
            "description": "Group escalated / unanswered questions by topic.",
            "requestBody": json_body(ref("GapsRequest")),
            "responses": {"200": json_resp("Report", ref("GapsReport"))}}},
        "/hint": {"post": {
            "operationId": "taskHint",
            "description": "Short practical hint for an intern stuck on a task (backend hint worker). KB-grounded; "
                           "502 on LLM failure, then the backend shows its own fallback text.",
            "requestBody": json_body({"type": "object", "required": ["title"], "properties": {
                "title": {"type": "string", "minLength": 1, "maxLength": 300},
                "description": {"type": ["string", "null"], "maxLength": 4000},
                "expected_result": {"type": ["string", "null"], "maxLength": 2000},
                "estimated_minutes": {"type": ["integer", "null"], "minimum": 1}}}),
            "responses": {"200": json_resp("Hint", {"type": "object", "required": ["hint"], "properties": {
                "hint": {"type": "string"},
                "sources": {"type": "array", "items": {"type": "string"}, "description": "KB document titles named in the hint"}}}),
                "502": json_resp("LLM unavailable", ERROR)}}},
        "/v1/kb/lint": {"get": {
            "operationId": "kbLint",
            "description": "Contradictions and stale documents in the knowledge base (cached; refreshed on reload).",
            "responses": {"200": json_resp("Report", ref("KbLintReport"))}}},
        "/v1/kb/docs/{doc_id}": {"get": {
            "operationId": "kbDocument",
            "parameters": [{"name": "doc_id", "in": "path", "required": True, "schema": {"type": "string"}}],
            "description": "One KB document (all sections) by the source id from chat events.",
            "responses": {"200": json_resp("Document", ref("KbDocument")), "404": json_resp("No such document", ERROR)}}},
        "/v1/suggestions": {"get": {
            "operationId": "suggestions",
            "description": "Ready-made questions (data/suggestions.json) whose document section exists in the current index, "
                           "each with a short excerpt of that section.",
            "responses": {"200": json_resp("Suggestions", {"type": "object", "required": ["suggestions"], "properties": {
                "suggestions": {"type": "array", "items": {"type": "object"}}}})}}},
        "/v1/kb/reload": {"post": {
            "operationId": "kbReload",
            "description": "Re-read the knowledge base from its source (Markdown dir or Confluence) and re-index.",
            "responses": {"200": json_resp("Reloaded", {
                "type": "object", "required": ["documents", "chunks"],
                "properties": {"documents": {"type": "integer"}, "chunks": {"type": "integer"},
                               "source": {"type": "string"}}}),
                          "502": json_resp("Source unavailable or empty; the previous index is kept", ERROR)}}},
        "/v1/admin/llm": {"get": {
            "operationId": "adminLlm",
            "description": "Provider, model, mode and API balance for the admin panel. OpenRouter: GET {base}/key; other "
                           "providers return a note. Never contains the key or a raw provider error.",
            "responses": {"200": json_resp("Status", ADMIN_LLM)}}},
        "/v1/admin/llm-ping": {"post": {
            "operationId": "adminLlmPing",
            "description": "Tiny completion (max_tokens 16) with the same base URL, key, model and extra body as chat; "
                           "canned result in mock mode. Failures are reported in the body (ok=false).",
            "responses": {"200": json_resp("Ping result", ADMIN_PING)}}},
        "/v1/admin/kb": {"get": {
            "operationId": "adminKb", "description": "Document and chunk counts of the current index.",
            "responses": {"200": json_resp("Counts", ADMIN_KB)}}},
        "/metrics": {"get": {
            "operationId": "metrics", "description": "Prometheus metrics (latency, tokens, finish reasons, errors).",
            "responses": {"200": {"description": "Prometheus text format",
                                  "content": {"text/plain": {"schema": {"type": "string"}}}}}}},
    },
    "components": {"schemas": {**deepcopy(SHARED), **deepcopy(AI_ONLY),
                               "AiStreamEvent": stream_events("AiActionEvent")}},
}

# ---------------------------------------------------------------- Backend spec
old_backend = json.loads((ROOT / "backend.openapi.json").read_text(encoding="utf-8"))
# Keep every path the backend team owns (auth, checklists, calendar, notes, ...); only the AI paths below are generated.
backend_paths = dict(old_backend["paths"])
backend_paths.update({
    "/api/chat": {"post": {
        "operationId": "chat", "deprecated": True, "security": BEARER,
        "description": "Legacy non-streaming chat. Use /api/chat/stream.",
        "requestBody": json_body(ref("ChatRequest")),
        "responses": {"200": json_resp("Success", ref("ChatResponse")),
                      "400": json_resp("Invalid request", ERROR), "401": json_resp("Unauthorized", ERROR),
                      "502": json_resp("AI unavailable", ERROR)}}},
    "/api/chat/stream": {"post": {
        "operationId": "chatStream", "security": BEARER,
        "description": "Streams the assistant answer. Backend: validates input, loads/creates conversation, "
                       "builds verified UserContext, calls AI /v1/chat/stream, re-emits events, persists both "
                       "messages, and converts the AI `action` (ActionProposal) into a stored PendingAction "
                       "emitted as `action` (BackendActionEvent). First event is `meta`. Event data schemas: "
                       "#/components/schemas/BackendStreamEvent.",
        "requestBody": json_body(ref("StreamChatRequest")),
        "responses": {"200": sse("Event stream"), "400": json_resp("Invalid request", ERROR),
                      "401": json_resp("Unauthorized", ERROR), "404": json_resp("Conversation not found", ERROR)}}},
    "/api/chat/messages": {"get": {
        "operationId": "chatMessages", "security": BEARER,
        "description": "Messages of a conversation (default: the user's latest), oldest first.",
        "parameters": [{"name": "conversation_id", "in": "query", "required": False,
                        "schema": {"type": "string", "format": "uuid"}}],
        "responses": {"200": json_resp("Success", {
            "type": "object", "required": ["conversation_id", "messages"],
            "properties": {"conversation_id": {"type": ["string", "null"]},
                           "messages": {"type": "array", "items": ref("ChatMessage")}}}),
            "401": json_resp("Unauthorized", ERROR)}}},
    "/api/chat/messages/{id}/feedback": {"post": {
        "operationId": "chatFeedback", "security": BEARER, "parameters": [UUID_PARAM],
        "requestBody": json_body(ref("Feedback")),
        "responses": {"204": {"description": "Saved"}, "404": json_resp("Not found", ERROR)}}},
    "/api/actions/{id}/confirm": {"post": {
        "operationId": "confirmAction", "security": BEARER, "parameters": [UUID_PARAM],
        "description": "Idempotent. Re-checks RBAC and params, executes once via the mock corporate service "
                       "adapter. Repeated calls return the same result.",
        "responses": {"200": json_resp("Executed (or already executed)", ref("ActionResult")),
                      "404": json_resp("Not found / not owner", ERROR),
                      "409": json_resp("Action cancelled", ERROR),
                      "422": json_resp("Invalid params", ERROR)}}},
    "/api/actions/{id}/cancel": {"post": {
        "operationId": "cancelAction", "security": BEARER, "parameters": [UUID_PARAM],
        "responses": {"200": json_resp("Cancelled", ref("PendingAction")),
                      "404": json_resp("Not found", ERROR), "409": json_resp("Already executed", ERROR)}}},
    "/api/suggestions": {"get": {
        "operationId": "chatSuggestions", "security": BEARER,
        "description": "Ready-made questions the chat shows as the placeholder of the empty input (AI /v1/suggestions). "
                       "Only entries whose document section exists in the knowledge base are returned.",
        "responses": {"200": json_resp("Suggestions", {"type": "object", "required": ["suggestions"], "properties": {
            "suggestions": {"type": "array", "items": {"type": "object", "required": ["question", "doc_id", "title", "heading", "url", "excerpt"], "properties": {
                "question": {"type": "string"}, "doc_id": {"type": "string"}, "title": {"type": "string"},
                "heading": {"type": "string"}, "url": {"type": "string"}, "excerpt": {"type": "string"}}}}}})}}},
    "/api/chat/hint": {"post": {
        "operationId": "chatHint", "security": BEARER,
        "description": "Answers one of the suggestions WITHOUT the model: a short excerpt of the document section plus a link. "
                       "The text is built by the backend from the knowledge base (the client sends only the question). Both messages "
                       "join the conversation, so the user's next message goes through the model with this exchange as history.",
        "requestBody": json_body({"type": "object", "required": ["question"], "properties": {
            "question": {"type": "string"}, "conversation_id": {"type": ["string", "null"], "format": "uuid"}}}),
        "responses": {"200": json_resp("Hint", {"type": "object", "required": ["conversation_id", "message_id", "text", "sources"], "properties": {
            "conversation_id": {"type": "string", "format": "uuid"}, "message_id": {"type": "string", "format": "uuid"},
            "text": {"type": "string"}, "sources": {"type": "array", "items": ref("Source")}}}),
                      "404": json_resp("Not one of the suggestions", ERROR)}}},
    "/api/auth/link": {"post": {
        "operationId": "linkSession",
        "description": "Opens a NEW session of the same user for another client (the Chrome extension next to the web app), "
                       "without the password: the caller proves possession of its own refresh token, which is not rotated. "
                       "Each client keeps its own refresh-token chain (rotation + replay detection would revoke a shared one). "
                       "Logging one client out does not sign the other out.",
        "requestBody": json_body({"type": "object", "required": ["refresh_token"], "properties": {"refresh_token": {"type": "string"}}}),
        "responses": {"201": json_resp("New session", {"type": "object", "required": ["access_token", "refresh_token", "expires_in"], "properties": {
            "access_token": {"type": "string"}, "refresh_token": {"type": "string"}, "token_type": {"type": "string"},
            "expires_in": {"type": "integer"}}}),
                      "401": json_resp("Invalid, expired, revoked or already rotated refresh token", ERROR)}}},
    "/api/requests": {"get": {
        "operationId": "listRequests", "security": BEARER,
        "description": "Current user's requests in corporate services (mock), newest first.",
        "responses": {"200": json_resp("Success", {"type": "array", "items": ref("ServiceRequest")})}},
        "post": {
            "operationId": "createRequest", "security": BEARER,
            "description": "Create a leave or access request from the form in the chat window. Submitting the form is the "
                           "user's explicit confirmation; validation and policy are the same as for a confirmed chat action "
                           "(e.g. interns never get admin access: 422).",
            "requestBody": json_body({"type": "object", "required": ["type", "params"], "properties": {
                "type": {"type": "string", "enum": ["leave_request", "access_request"]},
                "params": {"type": "object", "additionalProperties": {"type": "string"},
                           "description": "leave_request: kind (day_off|vacation), date_from, date_to, comment?; "
                                          "access_request: system, resource, access_level (read|write), justification"}}}),
            "responses": {"201": json_resp("Created", ref("ServiceRequest")),
                          "422": json_resp("Invalid params or not allowed by policy", ERROR)}}},
    "/api/requests/{id}": {"get": {
        "operationId": "getRequest", "security": BEARER, "parameters": [UUID_PARAM],
        "responses": {"200": json_resp("Success", ref("ServiceRequest")), "404": json_resp("Not found", ERROR)}}},
    "/api/buddy/questions": {"get": {
        "operationId": "buddyQuestions", "security": BEARER,
        "description": "Buddy only: questions escalated by the buddy's interns.",
        "parameters": [{"name": "status", "in": "query", "required": False,
                        "schema": {"type": "string", "enum": ["open", "answered"]}}],
        "responses": {"200": json_resp("Success", {"type": "array", "items": ref("MentorQuestion")}),
                      "403": json_resp("Not a buddy", ERROR)}}},
    "/api/buddy/questions/{id}/answer": {"post": {
        "operationId": "answerQuestion", "security": BEARER, "parameters": [UUID_PARAM],
        "requestBody": json_body(ref("AnswerMentorQuestion")),
        "responses": {"200": json_resp("Answered", ref("MentorQuestion")),
                      "403": json_resp("Not this buddy's intern", ERROR)}}},
    "/api/chat/brief/stream": {"post": {
        "operationId": "chatBrief", "security": BEARER,
        "description": "Proactive brief when the intern opens the chat (call at most once per session/day). "
                       "Backend builds UserContext, calls AI /v1/brief/stream and re-emits events like /api/chat/stream.",
        "responses": {"200": sse("Event stream"), "401": json_resp("Unauthorized", ERROR)}}},
    "/api/buddy/questions/{id}/draft": {"get": {
        "operationId": "questionDraft", "security": BEARER, "parameters": [UUID_PARAM],
        "description": "Buddy only: AI draft answer for an escalated question (AI /v1/mentor/draft).",
        "responses": {"200": json_resp("Draft", ref("MentorDraft")), "403": json_resp("Not this buddy's intern", ERROR),
                      "502": json_resp("AI unavailable", ERROR)}}},
    "/api/analytics/gaps": {"get": {
        "operationId": "knowledgeGaps", "security": BEARER,
        "description": "Buddy only: escalated questions grouped by topic (AI /v1/analytics/gaps).",
        "responses": {"200": json_resp("Report", ref("GapsReport")), "403": json_resp("Not a buddy", ERROR)}}},
    "/api/kb/lint": {"get": {
        "operationId": "kbLint", "security": BEARER,
        "description": "Buddy only: knowledge-base contradictions and stale documents (AI /v1/kb/lint).",
        "responses": {"200": json_resp("Report", ref("KbLintReport")), "403": json_resp("Not a buddy", ERROR)}}},
    "/api/kb/docs/{id}": {"get": {
        "operationId": "kbDocument", "security": BEARER,
        "parameters": [{"name": "id", "in": "path", "required": True, "schema": {"type": "string", "pattern": "^[A-Za-z0-9_.-]{1,128}$"}}],
        "description": "Any signed-in user: one knowledge-base document by the source id from chat events (AI /v1/kb/docs/{doc_id}). "
                       "Opened in the chat when the user clicks a source.",
        "responses": {"200": json_resp("Document", ref("KbDocument")), "404": json_resp("No such document", ERROR),
                      "502": json_resp("AI unavailable", ERROR)}}},
    "/api/questions": {"get": {
        "operationId": "ownQuestions", "security": BEARER,
        "description": "The intern's own questions escalated to the buddy, newest first (to notify about answers; "
                       "the answer is also added to the chat conversation the question came from).",
        "responses": {"200": json_resp("Success", {"type": "array", "items": ref("MentorQuestion")})}},
        "post": {
            "operationId": "createQuestion", "security": BEARER,
            "description": "Ask the buddy a question directly (the \"Вопросы наставнику\" tab), without the chat. "
                           "Same storage as a confirmed mentor_question action; the buddy sees it in the mentor desk.",
            "requestBody": json_body({"type": "object", "required": ["question"], "properties": {
                "question": {"type": "string", "minLength": 1, "maxLength": 2000}}}),
            "responses": {"201": json_resp("Created", ref("MentorQuestion")),
                          "422": json_resp("Empty question", ERROR)}}},
    "/api/buddy/requests": {"get": {
        "operationId": "staffRequests", "security": BEARER,
        "description": "Buddy/teamlead: requests of their interns, newest first.",
        "responses": {"200": json_resp("Success", {"type": "array", "items": {
            "type": "object", "required": ["request", "intern_id", "intern_name"],
            "properties": {"request": ref("ServiceRequest"), "intern_id": {"type": "string", "format": "uuid"},
                           "intern_name": {"type": "string"}}}}),
            "403": json_resp("Not a buddy", ERROR)}}},
    "/api/buddy/requests/{id}/decision": {"post": {
        "operationId": "decideRequest", "security": BEARER, "parameters": [UUID_PARAM],
        "description": "Mock corporate approval: the intern's buddy or teamlead approves or rejects a pending request. "
                       "The intern sees the new status in /api/requests, the brief and the extension.",
        "requestBody": json_body({"type": "object", "required": ["status"], "properties": {
            "status": {"type": "string", "enum": ["approved", "rejected"]}}}),
        "responses": {"200": json_resp("Decided", ref("ServiceRequest")), "400": json_resp("Invalid status", ERROR),
                      "404": json_resp("Not found / not this staff's intern", ERROR),
                      "409": json_resp("Already decided", ERROR)}}},
    "/api/admin/accounts": {"get": {
        "operationId": "adminAccounts", "security": BEARER,
        "description": "Teamlead only: all accounts with role, profession, team, buddy/teamlead names, last login and the "
                       "number of active sessions. Never returns password hashes, tokens or secrets.",
        "responses": {"200": json_resp("Accounts", {"type": "object", "required": ["accounts"], "properties": {
            "accounts": {"type": "array", "items": ADMIN_ACCOUNT}}}), "403": ADMIN_FORBIDDEN}}},
    "/api/admin/access": {"get": {
        "operationId": "adminAccess", "security": BEARER,
        "description": "Any signed-in user: is the admin panel open for this role (teamlead, or everyone with ADMIN_OPEN_FOR_ALL=1).",
        "responses": {"200": json_resp("Access", {"type": "object", "required": ["allowed"], "properties": {"allowed": {"type": "boolean"}}})}}},
    "/api/admin/feedback": {"get": {
        "operationId": "adminFeedback", "security": BEARER,
        "description": "Admin only: thumbs up/down counts under bot answers and the comments left under thumbs-down.",
        "responses": {"200": json_resp("Feedback", {"type": "object", "required": ["up", "down", "items"], "properties": {
            "up": {"type": "integer"}, "down": {"type": "integer"},
            "items": {"type": "array", "items": {"type": "object", "properties": {
                "message_id": {"type": "string"}, "comment": {"type": "string"}, "created_at": {"type": "string", "format": "date-time"}}}}}}),
            "403": json_resp("Forbidden", ERROR)}}},
    "/api/admin/llm": {"get": {
        "operationId": "adminLlm", "security": BEARER,
        "description": "Teamlead only: provider, model, mode and API balance (AI /v1/admin/llm).",
        "responses": {"200": json_resp("Status", ADMIN_LLM), "403": ADMIN_FORBIDDEN,
                      "502": json_resp("AI unavailable", ERROR)}}},
    "/api/admin/llm/ping": {"post": {
        "operationId": "adminLlmPing", "security": BEARER,
        "description": "Teamlead only: a tiny completion to check the model (AI /v1/admin/llm-ping).",
        "responses": {"200": json_resp("Ping result", ADMIN_PING), "403": ADMIN_FORBIDDEN,
                      "502": json_resp("AI unavailable", ERROR)}}},
    "/api/admin/system": {"get": {
        "operationId": "adminSystem", "security": BEARER,
        "description": "Teamlead only: health checks (database, AI service, knowledge base, hint worker) and counters.",
        "responses": {"200": json_resp("Status", ADMIN_SYSTEM), "403": ADMIN_FORBIDDEN}}},
    "/api/admin/kb/reload": {"post": {
        "operationId": "adminKbReload", "security": BEARER,
        "description": "Teamlead only: re-read the knowledge base (AI /v1/kb/reload).",
        "responses": {"200": json_resp("Reloaded", ADMIN_KB), "403": ADMIN_FORBIDDEN,
                      "502": json_resp("AI unavailable", ERROR)}}},
    "/api/admin/notifications": {"get": {
        "operationId": "adminNotifications", "security": BEARER,
        "description": "Teamlead only: catalog of Бади's pop-up notifications (single source of truth) with live numbers "
                       "for backend-backed entries.",
        "responses": {"200": json_resp("Catalog", {"type": "object", "required": ["items"], "properties": {
            "items": {"type": "array", "items": ADMIN_NOTIFICATION}}}), "403": ADMIN_FORBIDDEN}}},
    "/api/admin/notifications/stuck_task/check": {"post": {
        "operationId": "adminCheckStuckTask", "security": BEARER,
        "description": "Teamlead only: run the real hint worker tick now and report overdue tasks, hints before/after "
                       "and up to 10 affected tasks.",
        "responses": {"200": json_resp("Result", ADMIN_STUCK_CHECK), "403": ADMIN_FORBIDDEN}}},
    "/api/metrics/chat": {"get": {
        "operationId": "chatMetrics", "security": BEARER,
        "description": "Buddy only: aggregated assistant metrics for own interns.",
        "responses": {"200": json_resp("Success", ref("ChatMetrics"))}}},
})
backend_spec = {
    "openapi": "3.1.0",
    "info": {"title": "AI-buddy backend (public)", "version": "0.2.0",
             "description": "Backend-owned paths are kept as they are in this file; AI/chat paths are generated by "
                            "contracts/build_contracts.py."},
    "servers": old_backend.get("servers", [{"url": "http://127.0.0.1:8080"}]),
    "paths": backend_paths,
    "components": {
        "securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}},
        "schemas": {**deepcopy(SHARED), **deepcopy(BACKEND_ONLY),
                    "BackendStreamEvent": stream_events("BackendActionEvent", extra=("MetaEvent",))},
    },
}

for name, spec in (("ai", ai_spec), ("backend", backend_spec)):
    (ROOT / f"{name}.openapi.json").write_text(json.dumps(spec, ensure_ascii=False, indent=2) + "\n",
                                               encoding="utf-8", newline="\n")
print("contracts written")
