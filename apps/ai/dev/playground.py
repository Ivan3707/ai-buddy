"""DEV playground: a stand-in for the Go backend's AI endpoints, so the whole bot can be tried by hand
today (extension side panel + a small mentor page) while the real backend endpoints are being built.

NOT production code: in-memory state, one demo intern, any password accepted. It follows
contracts/backend.openapi.json, so the extension behaves exactly as it will with the real backend.

    docker compose -p ai-buddy --profile playground up -d --build playground
    # extension: BACKEND_ORIGIN=http://127.0.0.1:18090 npm run build -w @ai-buddy/extension
    # mentor page: http://127.0.0.1:18090/
    # intern chat:  http://127.0.0.1:18090/intern
"""
from __future__ import annotations

import json
import os
import uuid
from datetime import date, datetime, timezone
from typing import Any

import httpx
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse, Response, StreamingResponse

AI = os.getenv("AI_URL", "http://ai:8001").rstrip("/")
TOKEN = "playground-token"
app = FastAPI(title="AI-buddy playground (dev only)")


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


USERS = {
    "intern": {"id": "00000000-0000-4000-8000-0000000000a1", "email": "intern@demo.local", "role": "intern",
               "profession": "ml", "display_name": "Аня", "created_at": now()},
    "buddy": {"id": "00000000-0000-4000-8000-0000000000b1", "email": "buddy@demo.local", "role": "buddy",
              "profession": None, "display_name": "Олег", "created_at": now()},
}
STATE: dict[str, Any] = {
    "conversations": {}, "actions": {}, "results": {}, "requests": [], "questions": [], "feedback": {}, "counter": 0,
    "tasks": [
        {"id": "t1", "title": "Настроить VPN и рабочее окружение", "status": "completed", "checklist": "common",
         "stage_title": "Первая неделя", "review_comment": "Добавь ссылку на ноутбук в MLflow"},
        {"id": "t5", "title": "Воспроизвести baseline модели рекомендаций", "status": "active",
         "checklist": "professional", "stage_title": "Погружение", "requires_result": True},
        {"id": "t6", "title": "Изучить дашборды команды в Grafana", "status": "unavailable",
         "checklist": "professional", "stage_title": "Погружение"},
    ],
}


def user_of(request: Request) -> dict[str, Any]:
    auth = request.headers.get("authorization", "")
    if not auth.startswith(f"Bearer {TOKEN}"):
        raise HTTPException(401, "unauthorized")
    return USERS["buddy" if auth.endswith(":buddy") else "intern"]


def user_context() -> dict[str, Any]:
    u = USERS["intern"]
    return {"user_id": u["id"], "display_name": u["display_name"], "role": "intern", "profession": "ml",
            "team": "Рекомендации", "buddy_name": "Олег", "today": date.today().isoformat(),
            "current_stage": {"title": "Погружение", "position": 2, "total": 4},
            "tasks": STATE["tasks"], "open_requests": STATE["requests"][-20:],
            "allowed_actions": ["leave_request", "access_request", "mentor_question"]}


# ------------------------------------------------------------------ auth
@app.post("/api/auth/login")
async def login(request: Request):
    body = await request.json()
    who = "buddy" if str(body.get("email", "")).startswith("buddy") else "intern"
    return {"access_token": f"{TOKEN}:{who}", "token_type": "Bearer", "user": USERS[who]}


@app.get("/api/auth/me")
async def me(request: Request):
    return user_of(request)


# ------------------------------------------------------------------ chat
def sse(event: str, data: dict[str, Any]) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"


async def relay(path: str, payload: dict[str, Any], conversation_id: str):
    """Call AI, re-emit its SSE, add `meta`, turn ActionProposal into a stored PendingAction."""
    message_id = str(uuid.uuid4())
    yield sse("meta", {"conversation_id": conversation_id, "message_id": message_id})
    text, event = "", None
    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(10, read=90)) as client:
            async with client.stream("POST", f"{AI}{path}", json=payload) as resp:
                async for line in resp.aiter_lines():
                    if line.startswith("event: "):
                        event = line[7:]
                    elif line.startswith("data: "):
                        data = json.loads(line[6:])
                        if event == "delta":
                            text += data["text"]
                        if event == "action":
                            p = data["proposal"]
                            action = {"id": str(uuid.uuid4()), "type": p["type"], "params": p["params"],
                                      "summary": p["summary"], "status": "pending", "created_at": now()}
                            STATE["actions"][action["id"]] = action
                            data = {"action": action}
                        yield sse(event, data)
    except httpx.HTTPError:
        yield sse("error", {"code": "llm_unavailable", "message": "AI-сервис недоступен"})
    STATE["conversations"].setdefault(conversation_id, []).append({"role": "assistant", "content": text[:8000]})


@app.post("/api/chat/stream")
async def chat_stream(request: Request):
    user_of(request)
    body = await request.json()
    cid = body.get("conversation_id") or str(uuid.uuid4())
    history = STATE["conversations"].setdefault(cid, [])
    payload = {"message": body["message"], "history": history[-20:], "user_context": user_context()}
    if body.get("page_context"):
        payload["page_context"] = body["page_context"]
    history.append({"role": "user", "content": body["message"]})
    STATE["counter"] += 1
    return StreamingResponse(relay("/v1/chat/stream", payload, cid), media_type="text/event-stream")


@app.post("/api/chat/brief/stream")
async def brief(request: Request):
    user_of(request)
    return StreamingResponse(relay("/v1/brief/stream", {"user_context": user_context()}, str(uuid.uuid4())),
                             media_type="text/event-stream")


@app.post("/api/chat")
async def legacy_chat(request: Request):
    user_of(request)
    async with httpx.AsyncClient(timeout=30) as client:
        r = await client.post(f"{AI}/chat", json=await request.json())
    return JSONResponse(r.json(), status_code=r.status_code)


@app.post("/api/chat/messages/{message_id}/feedback")
async def feedback(message_id: str, request: Request):
    user_of(request)
    STATE["feedback"][message_id] = (await request.json()).get("rating")
    return Response(status_code=204)


# ------------------------------------------------------------------ actions (mock corporate services)
APPROVER = {"leave_request": "Руководитель команды (демо)", "access_request": "Владелец ресурса (демо)"}


@app.post("/api/actions/{action_id}/confirm")
async def confirm(action_id: str, request: Request):
    user_of(request)
    action = STATE["actions"].get(action_id)
    if action is None:
        raise HTTPException(404, "not found")
    if action["status"] == "cancelled":
        return JSONResponse({"error": "action cancelled"}, status_code=409)
    if action_id in STATE["results"]:
        return STATE["results"][action_id]  # idempotent
    if action["type"] == "access_request" and action["params"].get("access_level") == "admin":
        # backend policy layer: interns never get admin, whatever the model proposed
        action["status"] = "failed"
        return JSONResponse({"error": "Стажёрам доступ admin не выдаётся"}, status_code=422)
    result: dict[str, Any] = {"action": action}
    if action["type"] == "mentor_question":
        q = {"id": str(uuid.uuid4()), "intern_id": USERS["intern"]["id"], "intern_name": "Аня",
             "question": action["params"]["question"], "context": action["params"].get("context", ""),
             "status": "open", "escalation_level": "mentor", "created_at": now()}
        STATE["questions"].append(q)
        result["mentor_question"] = q
    else:
        prefix = "ABS" if action["type"] == "leave_request" else "ACC"
        req = {"id": str(uuid.uuid4()), "number": f"{prefix}-{len(STATE['requests']) + 1:06d}",
               "type": action["type"], "status": "pending_approval", "approver": APPROVER[action["type"]],
               "params": action["params"], "created_at": now(), "updated_at": now()}
        STATE["requests"].append(req)
        result["request"] = req
    action["status"] = "executed"
    STATE["results"][action_id] = result
    return result


@app.post("/api/actions/{action_id}/cancel")
async def cancel(action_id: str, request: Request):
    user_of(request)
    action = STATE["actions"].get(action_id)
    if action is None:
        raise HTTPException(404, "not found")
    if action["status"] == "executed":
        return JSONResponse({"error": "already executed"}, status_code=409)
    action["status"] = "cancelled"
    return action


@app.get("/api/requests")
async def requests_list(request: Request):
    user_of(request)
    return list(reversed(STATE["requests"]))


@app.post("/dev/requests/{number}/{status}")
async def set_request_status(number: str, status: str):
    """Mentor page button: approve/reject a mock request to see the status change in chat and brief."""
    for r in STATE["requests"]:
        if r["number"] == number:
            r["status"], r["updated_at"] = status, now()
            return r
    raise HTTPException(404, "not found")


# ------------------------------------------------------------------ mentor side (B3, B4, B5)
@app.get("/api/buddy/questions")
async def buddy_questions():
    return list(reversed(STATE["questions"]))


@app.get("/api/buddy/questions/{qid}/draft")
async def buddy_draft(qid: str):
    q = next((x for x in STATE["questions"] if x["id"] == qid), None)
    if q is None:
        raise HTTPException(404, "not found")
    async with httpx.AsyncClient(timeout=90) as client:
        r = await client.post(f"{AI}/v1/mentor/draft", json={"question": q["question"], "context": q["context"] or "",
                                                              "user_context": user_context()})
    return JSONResponse(r.json(), status_code=r.status_code)


@app.post("/api/buddy/questions/{qid}/answer")
async def buddy_answer(qid: str, request: Request):
    q = next((x for x in STATE["questions"] if x["id"] == qid), None)
    if q is None:
        raise HTTPException(404, "not found")
    q.update(status="answered", answer=(await request.json())["answer"], answered_at=now())
    return q


@app.get("/api/analytics/gaps")
async def gaps():
    questions = [{"id": q["id"], "text": q["question"]} for q in STATE["questions"]]
    async with httpx.AsyncClient(timeout=90) as client:
        r = await client.post(f"{AI}/v1/analytics/gaps", json={"questions": questions})
    return JSONResponse(r.json(), status_code=r.status_code)


@app.get("/api/kb/lint")
async def lint():
    async with httpx.AsyncClient(timeout=180) as client:
        r = await client.get(f"{AI}/v1/kb/lint")
    return JSONResponse(r.json(), status_code=r.status_code)


@app.get("/api/metrics/chat")
async def chat_metrics():
    escalated = len(STATE["questions"])
    total = STATE["counter"]
    ups = sum(v == "up" for v in STATE["feedback"].values())
    return {"questions_total": total, "answered_by_bot": max(total - escalated, 0), "escalated_to_mentor": escalated,
            "deflection_rate": round(1 - escalated / total, 2) if total else None, "thumbs_up": ups,
            "thumbs_down": sum(v == "down" for v in STATE["feedback"].values()),
            "actions_executed": sum(a["status"] == "executed" for a in STATE["actions"].values()),
            "estimated_mentor_minutes_saved": max(total - escalated, 0) * 5}


@app.get("/", response_class=HTMLResponse)
async def mentor_page():
    return MENTOR_PAGE


@app.get("/demo/{name}", response_class=HTMLResponse)
async def demo_page(name: str):
    """Fake wiki / dashboards / task board where the extension's on-page character is shown."""
    from .demo_pages import PAGES
    if name not in PAGES:
        raise HTTPException(404, "not found")
    return PAGES[name]


INTERN_PAGE = os.path.join(os.path.dirname(__file__), "intern.html")


@app.get("/intern", response_class=HTMLResponse)
async def intern_page():
    """Intern chat in a plain browser tab: same backend API as the extension, no install needed."""
    with open(INTERN_PAGE, encoding="utf-8") as f:
        return f.read()


MENTOR_PAGE = """<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Плейграунд наставника</title><style>
body{font:15px/1.5 system-ui,sans-serif;max-width:900px;margin:24px auto;padding:0 16px;color:#1d2330;background:#f7f8fa}
h1{font-size:22px}h2{font-size:17px;margin-top:28px}section{background:#fff;border:1px solid #e1e4ea;border-radius:10px;padding:14px 16px;margin:12px 0}
button{font:inherit;padding:4px 10px;border-radius:6px;border:1px solid #c5cad3;background:#fff;cursor:pointer}button:hover{background:#eef1f6}
.muted{color:#68708a;font-size:13px}pre{white-space:pre-wrap;background:#f2f4f7;padding:8px;border-radius:6px}.tag{font-size:12px;padding:1px 6px;border-radius:10px;background:#eef1f6}
textarea{width:100%;min-height:90px;font:inherit}</style></head><body>
<h1>Плейграунд наставника <span class="tag">демо, dev-only</span></h1>
<p class="muted">Заменитель кабинета наставника, пока backend не готов. Данные в памяти, сбрасываются при перезапуске. Чат стажёра — <a href="/intern" target="_blank">/intern</a>.</p>
<h2>Метрики чата</h2><section id="metrics">…</section>
<h2>Заявки стажёра</h2><section id="requests">…</section>
<h2>Вопросы, переданные наставнику</h2><div id="questions">…</div>
<h2>Пробелы базы знаний <button onclick="gaps()">Посчитать</button></h2><section id="gaps" class="muted">Нажми «Посчитать».</section>
<h2>Линтер базы знаний <button onclick="lint()">Проверить</button></h2><section id="lint" class="muted">Нажми «Проверить» (до минуты: модель проверяет пары документов).</section>
<script>
const H={"Authorization":"Bearer playground-token:buddy","Content-Type":"application/json"};
const esc=s=>String(s??"").replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));
async function j(u,o={}){const r=await fetch(u,{headers:H,...o});return r.json()}
async function load(){
 const m=await j("/api/metrics/chat");document.getElementById("metrics").innerHTML=
  `Вопросов: <b>${m.questions_total}</b> · решено ботом: <b>${m.answered_by_bot}</b> · передано наставнику: <b>${m.escalated_to_mentor}</b> · deflection: <b>${m.deflection_rate??"—"}</b> · 👍 ${m.thumbs_up} 👎 ${m.thumbs_down} · действий: ${m.actions_executed} · сэкономлено ≈ <b>${m.estimated_mentor_minutes_saved} мин</b>`;
 const rs=await j("/api/requests");document.getElementById("requests").innerHTML=rs.length?rs.map(r=>
  `<div><b>${esc(r.number)}</b> ${esc(r.type)} · <span class="tag">${esc(r.status)}</span> ${esc(JSON.stringify(r.params))}
   <button onclick="setStatus('${r.number}','approved')">Согласовать</button> <button onclick="setStatus('${r.number}','rejected')">Отклонить</button></div>`).join(""):"<span class=muted>Пока нет.</span>";
 const qs=await j("/api/buddy/questions");document.getElementById("questions").innerHTML=qs.length?qs.map(q=>
  `<section><b>${esc(q.intern_name)}:</b> ${esc(q.question)} <span class="tag">${esc(q.status)}</span>
   <div class="muted">${esc(q.context)}</div>${q.answer?`<pre>${esc(q.answer)}</pre>`:
   `<button onclick="draft('${q.id}')">AI-черновик ответа</button><div id="d-${q.id}"></div>`}</section>`).join(""):"<section class=muted>Пока нет.</section>";
}
async function setStatus(n,s){await fetch(`/dev/requests/${n}/${s}`,{method:"POST"});load()}
async function draft(id){const el=document.getElementById("d-"+id);el.textContent="Готовлю черновик…";
 const d=await j(`/api/buddy/questions/${id}/draft`);
 el.innerHTML=`<p class=muted>Уверенность: <b>${esc(d.confidence)}</b> · источники: ${(d.sources||[]).map(s=>esc(s.title)).join("; ")||"—"}</p>
  <textarea id="a-${id}">${esc(d.draft)}</textarea><button onclick="answer('${id}')">Отправить стажёру</button>
  ${d.kb_suggestion?`<p class=muted>Предложение статьи в базу: <b>${esc(d.kb_suggestion.title)}</b></p><pre>${esc(d.kb_suggestion.text)}</pre>`:""}`}
async function answer(id){await j(`/api/buddy/questions/${id}/answer`,{method:"POST",body:JSON.stringify({answer:document.getElementById("a-"+id).value})});load()}
async function gaps(){const el=document.getElementById("gaps");el.textContent="Считаю…";const g=await j("/api/analytics/gaps");
 el.innerHTML=g.clusters.length?`Всего вопросов: ${g.total_questions}<ul>${g.clusters.map(c=>`<li><b>${esc(c.label)}</b> — ${c.count}: ${c.examples.map(esc).join(" / ")}</li>`).join("")}</ul>`:"Эскалированных вопросов пока нет."}
async function lint(){const el=document.getElementById("lint");el.textContent="Проверяю…";const r=await j("/api/kb/lint");
 el.innerHTML=r.issues.length?`<ul>${r.issues.map(i=>`<li><span class=tag>${esc(i.kind)}</span> <b>${i.doc_ids.map(esc).join(" ↔ ")}</b>: ${esc(i.detail)}</li>`).join("")}</ul>`:"Проблем не найдено."}
load();setInterval(load,5000);
</script></body></html>"""
