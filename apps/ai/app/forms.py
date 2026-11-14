"""Forms that the assistant shows in the chat instead of asking for details in text.

When the user wants a request, a note, a calendar event or a question to the mentor, and some details are missing,
the model calls `show_form` with what it already knows. The chat draws a small form (prefilled), the user fills in the
rest and submits it. Nothing is created by the model: the form is submitted by the user to the ordinary backend API,
which validates everything again. Here we only make sure that the model cannot push anything odd into the form.
"""
from __future__ import annotations

import json
import re
from typing import Any

from .llm import ToolCall
from .prompts import ACCESS_LEVELS_BY_ROLE

ACCESS_SYSTEMS = ["jira", "confluence", "gitlab", "grafana", "redash", "kubernetes"]

DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
DATETIME = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$")

# form type -> (action that must be allowed, or None when every user may use it)
FORM_ACTION = {"leave_request": "leave_request", "access_request": "access_request", "mentor_question": "mentor_question",
               "note": None, "event": None}

# field -> kind of value: ("enum", [...]) | ("text", max_length) | ("date",) | ("datetime",)
FIELDS: dict[str, dict[str, tuple]] = {
    "leave_request": {"kind": ("enum", ["day_off", "vacation"]), "date_from": ("date",), "date_to": ("date",), "comment": ("text", 300)},
    "access_request": {"system": ("enum", ACCESS_SYSTEMS), "resource": ("text", 200), "access_level": ("enum", ["read", "write"]),
                       "justification": ("text", 500)},
    "note": {"title": ("text", 160), "content": ("text", 8000)},
    "event": {"title": ("text", 160), "description": ("text", 2000), "starts_at": ("datetime",), "ends_at": ("datetime",)},
    "mentor_question": {"question": ("text", 2000)},
}


def allowed_forms(allowed_actions: set[str] | list[str]) -> list[str]:
    allowed = set(allowed_actions)
    return [form for form, action in FORM_ACTION.items() if action is None or action in allowed]


def sanitize_prefill(form: str, raw: Any, role: str = "intern") -> dict[str, str]:
    """Keeps only known fields with valid values. Unknown fields, wrong enums and malformed dates are dropped silently."""
    if not isinstance(raw, dict):
        return {}
    out: dict[str, str] = {}
    for name, spec in FIELDS.get(form, {}).items():
        value = raw.get(name)
        if not isinstance(value, str) or not value.strip():
            continue
        value = value.strip()
        kind = spec[0]
        if kind == "enum":
            options = ACCESS_LEVELS_BY_ROLE.get(role, ["read"]) if name == "access_level" else spec[1]
            if value in options:
                out[name] = value
        elif kind == "date":
            if DATE.match(value):
                out[name] = value
        elif kind == "datetime":
            value = value[:16]
            if DATETIME.match(value):
                out[name] = value
        else:
            out[name] = value[: spec[1]]
    return out


def to_form(call: ToolCall, allowed: list[str], role: str = "intern") -> dict[str, Any] | None:
    """A validated {type, prefill} from a show_form tool call, or None when the call is not usable."""
    if call.name != "show_form":
        return None
    try:
        args = json.loads(call.arguments or "{}")
    except ValueError:
        return None
    form = args.get("form") if isinstance(args, dict) else None
    if form not in allowed:
        return None
    return {"type": form, "prefill": sanitize_prefill(form, args.get("prefill"), role)}
