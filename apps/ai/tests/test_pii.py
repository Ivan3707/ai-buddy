"""Personal data masking (A4)."""
from app.agent import mask_request
from app.pii import luhn_ok, mask
from app.schemas import AgentChatRequest
from conftest import user_context


def test_masks_card_passport_phone_snils():
    text, found = mask("карта 4111 1111 1111 1111, паспорт 4510 123456, тел +7 (912) 345-67-89, СНИЛС 112-233-445 95")
    assert "4111" not in text and "123456" not in text and "345-67" not in text and "445 95" not in text
    assert text.count("скрыт") == 4
    assert dict(found) == {"card": 1, "passport": 1, "phone": 1, "snils": 1}


def test_keeps_ordinary_numbers():
    for s in ("отпуск с 1 по 14 декабря", "заявка ABS-000123", "стендап в 10:30", "заказ 1234567890123"):
        assert mask(s) == (s, {}), s
    assert luhn_ok("4111111111111111") and not luhn_ok("1234567890123")


def test_request_masking_covers_message_and_user_history_only():
    req = AgentChatRequest(message="мой номер 89123456789", user_context=user_context(), history=[
        {"role": "user", "content": "паспорт 4510123456"}, {"role": "assistant", "content": "ABS 4510123456"}])
    masked, found = mask_request(req)
    assert masked.message == "мой номер [телефон скрыт]"
    assert masked.history[0].content == "паспорт [паспорт скрыт]"
    assert masked.history[1].content == "ABS 4510123456"  # assistant text is ours
    assert sum(found.values()) == 2 and req.message == "мой номер 89123456789"
