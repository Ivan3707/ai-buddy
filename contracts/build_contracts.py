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
    "DoneEvent": {
        "type": "object", "required": ["finish_reason", "mode"],
        "properties": {
            "finish_reason": {"type": "string", "enum": ["answered", "no_answer", "action_proposed"],
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
                 "properties": {"rating": {"type": "string", "enum": ["up", "down"]}}},
    "AnswerMentorQuestion": {"type": "object", "additionalProperties": False, "required": ["answer"],
                             "properties": {"answer": {"type": "string", "minLength": 1, "maxLength": 4000}}},
    "ChatMetrics": {
        "type": "object", "description": "Product metrics for the demo dashboard / presentation",
        "properties": {
            "questions_total": {"type": "integer"},
            "answered_by_bot": {"type": "integer"},
            "escalated_to_mentor": {"type": "integer"},
            "deflection_rate": {"type": "number", "description": "1 - escalated/questions"},
            "thumbs_up": {"type": "integer"}, "thumbs_down": {"type": "integer"},
            "actions_executed": {"type": "integer"},
            "estimated_mentor_minutes_saved": {"type": "number"},
        },
    },
}


def stream_events(action_schema, extra=()):
    names = ["StatusEvent", "DeltaEvent", "SourcesEvent", action_schema, "DoneEvent", "ErrorEvent", *extra]
    return {"oneOf": [ref(n) for n in names]}


def sse(description):
    return {"description": description,
            "content": {"text/event-stream": {"schema": {"type": "string",
                                                          "description": SSE_DESCRIPTION}}}}


BEARER = [{"bearerAuth": []}]
UUID_PARAM = {"name": "id", "in": "path", "required": True, "schema": {"type": "string", "format": "uuid"}}

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
        "/v1/kb/reload": {"post": {
            "operationId": "kbReload",
            "description": "Re-read the knowledge base from its source (Markdown dir or Confluence) and re-index.",
            "responses": {"200": json_resp("Reloaded", {
                "type": "object", "required": ["documents", "chunks"],
                "properties": {"documents": {"type": "integer"}, "chunks": {"type": "integer"},
                               "source": {"type": "string"}}}),
                          "502": json_resp("Source unavailable or empty; the previous index is kept", ERROR)}}},
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
    "/api/requests": {"get": {
        "operationId": "listRequests", "security": BEARER,
        "description": "Current user's requests in corporate services (mock), newest first.",
        "responses": {"200": json_resp("Success", {"type": "array", "items": ref("ServiceRequest")})}}},
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
    "/api/questions": {"get": {
        "operationId": "ownQuestions", "security": BEARER,
        "description": "The intern's own questions escalated to the buddy, newest first (to notify about answers; "
                       "the answer is also added to the chat conversation the question came from).",
        "responses": {"200": json_resp("Success", {"type": "array", "items": ref("MentorQuestion")})}}},
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
