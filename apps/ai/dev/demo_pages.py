"""DEV ONLY: three fake "corporate" pages for showing the on-page character (extension) in a demo.

They live on the playground origin, which the extension already has host permission for, so the character
appears there without any permission prompt. Synthetic content, labelled as demo — not Avito's tools or rules.
"""
from __future__ import annotations

NAV = """<nav><a href="/demo/wiki">Вики</a><a href="/demo/grafana">Дашборды</a><a href="/demo/jira">Задачи</a>
<span class="demo">демо-страницы, данные синтетические</span></nav>"""

BASE_CSS = """
:root{--bg:#f4f5f7;--card:#fff;--ink:#1d2330;--muted:#68708a;--line:#e1e4ea;--accent:#3b6fd8}
@media (prefers-color-scheme:dark){:root{--bg:#121417;--card:#1b1e23;--ink:#e6e8ee;--muted:#9aa2b5;--line:#2a2f37;--accent:#7aa2ff}}
*{box-sizing:border-box}body{margin:0;font:15px/1.55 system-ui,-apple-system,"Segoe UI",sans-serif;background:var(--bg);color:var(--ink)}
nav{display:flex;gap:16px;align-items:center;padding:10px 24px;background:var(--card);border-bottom:1px solid var(--line);flex-wrap:wrap}
nav a{color:var(--accent);text-decoration:none;font-weight:600}.demo{margin-left:auto;font-size:12px;color:var(--muted)}
main{max-width:1100px;margin:0 auto;padding:24px 16px}h1{font-size:24px;margin:0 0 4px}.muted{color:var(--muted);font-size:13px}
.card{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:14px 16px}
"""

WIKI = f"""<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Вики команды — памятка по мержу (демо)</title><style>{BASE_CSS}
article{{max-width:760px}}article h2{{font-size:18px;margin-top:24px}}mark{{background:#fff3b0;padding:0 2px}}
@media (prefers-color-scheme:dark){{mark{{background:#5a4a00;color:#fff}}}}</style></head><body>{NAV}
<main><article class="card"><h1>Памятка по мержу</h1><p class="muted">Вики команды «Рекомендации» · обновлено 12.02.2025 · демо</p>
<h2>Перед мержем</h2>
<p>Каждый merge request проходит ревью. <mark>Для мержа нужно минимум 2 одобрения</mark> от участников команды,
одно из них — от владельца сервиса.</p>
<p>Перед тем как просить ревью, прогоните тесты локально и убедитесь, что CI зелёный.</p>
<h2>Ветки</h2><p>Ветку называйте по номеру задачи: <code>REC-123-short-name</code>. После мержа удаляйте ветку.</p>
<h2>Если ревью затянулось</h2><p>Напомните ревьюеру в рабочем канале. Если ответа нет больше суток — напишите наставнику.</p>
<p class="muted">Подсказка для демо: выделите жёлтую фразу — Бади предложит проверить, актуальна ли она.</p>
</article></main></body></html>"""


def _spark(points: list[int], color: str) -> str:
    step = 280 / (len(points) - 1)
    path = " ".join(f"{'M' if i == 0 else 'L'}{i * step:.0f},{70 - p}" for i, p in enumerate(points))
    return f'<svg viewBox="0 0 280 80" width="100%" height="80"><path d="{path}" fill="none" stroke="{color}" stroke-width="2.5"/></svg>'


PANELS = [
    ("Latency p95, мс", "182", _spark([30, 32, 31, 35, 42, 38, 36, 50, 47, 40, 39, 41], "#e0a030")),
    ("RPS", "1 240", _spark([40, 44, 46, 45, 50, 55, 58, 54, 52, 57, 60, 59], "#3fb27f")),
    ("Ошибки 5xx, %", "0.4", _spark([5, 6, 5, 4, 8, 20, 9, 6, 5, 5, 4, 5], "#e05a5a")),
    ("CTR онлайн, %", "3.1", _spark([30, 31, 33, 32, 34, 33, 35, 36, 35, 37, 36, 38], "#5b8def")),
]

GRAFANA = f"""<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Grafana — Рекомендации: онлайн-метрики (демо)</title><style>{BASE_CSS}
.grid{{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:12px;margin-top:16px}}
.value{{font-size:28px;font-weight:700}}.label{{color:var(--muted);font-size:13px}}</style></head><body>{NAV}
<main><h1>Рекомендации — онлайн-метрики</h1><p class="muted">Дашборд метрик сервиса (демо, данные синтетические) · последние 6 часов</p>
<div class="grid">{''.join(f'<div class="card"><div class="label">{t}</div><div class="value">{v}</div>{s}</div>' for t, v, s in PANELS)}</div>
</main></body></html>"""

TICKETS = {
    "К работе": [("REC-214", "Воспроизвести baseline модели рекомендаций"), ("REC-219", "Добавить метрику coverage в отчёт")],
    "В работе": [("REC-205", "Обновить фичи пользователя в витрине")],
    "Ревью": [("REC-198", "Ускорить инференс ранжирования")],
    "Готово": [("REC-190", "Настроить алерт на рост 5xx")],
}

JIRA = f"""<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Jira — доска REC (демо)</title><style>{BASE_CSS}
.board{{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;margin-top:16px}}
.col{{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:10px}}.col h2{{font-size:13px;text-transform:uppercase;color:var(--muted);margin:0 0 8px}}
.t{{border:1px solid var(--line);border-radius:8px;padding:8px;margin-bottom:8px;font-size:14px}}.t b{{color:var(--accent);font-size:12px;display:block}}</style></head><body>{NAV}
<main><h1>Доска команды «Рекомендации»</h1><p class="muted">Трекер задач (демо, данные синтетические)</p>
<div class="board">{''.join(f'<div class="col"><h2>{col}</h2>' + ''.join(f'<div class="t"><b>{k}</b>{t}</div>' for k, t in items) + '</div>' for col, items in TICKETS.items())}</div>
</main></body></html>"""

PAGES = {"wiki": WIKI, "grafana": GRAFANA, "jira": JIRA}
