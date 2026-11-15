import { completeItem, submitItem, type ChecklistItem } from '@ai-buddy/client';

/** Следующий этап задачи: с проверкой уходит наставнику (нужен комментарий или ссылка), без проверки завершается. */
export async function advanceItem(token: string, item: ChecklistItem, result?: { text?: string; url?: string }): Promise<void> {
  if (item.requires_review) {
    const text = result?.text?.trim() || null;
    const url = result?.url?.trim() || null;
    if (!text && !url) throw new Error('Добавьте комментарий или ссылку на результат');
    await submitItem('', token, item.id, { result_text: text, result_url: url });
  } else {
    await completeItem('', token, item.id);
  }
}
