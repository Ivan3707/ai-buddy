import { chat } from './client.js';
const form = document.querySelector('#chat');
const output = document.querySelector('#reply');
form.addEventListener('submit', async event => {
  event.preventDefault();
  const message = document.querySelector('#message').value;
  if (!message.trim()) return;
  const button = form.querySelector('button');
  button.disabled = true;
  output.textContent = 'Ищем…';
  try {
    const result = await chat('http://127.0.0.1:8080', message);
    output.textContent = result.answer;
    for (const source of result.sources) {
      const link = document.createElement('a');
      const url = new URL(source.url);
      if (url.protocol !== 'https:') continue;
      link.href = url.href; link.textContent = `${source.title} (демо)`;
      link.target = '_blank'; link.rel = 'noreferrer';
      output.append(document.createElement('br'), link);
    }
  } catch { output.textContent = 'Backend недоступен. Запустите локальный стенд.'; }
  finally { button.disabled = false; }
});
