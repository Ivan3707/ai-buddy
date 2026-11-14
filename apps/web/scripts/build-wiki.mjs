// Builds the demo corporate wiki (static HTML) from the synthetic knowledge base data/kb/*.md.
// The bot cites these pages as sources: "Открыть документ полностью" leads here.
// Output: apps/web/public/wiki/<id>.html + index.html (generated, not in git). Synthetic data, labelled as demo.
import { mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';

const kbDir = new URL(process.env.KB_DIR ? `file:///${process.env.KB_DIR.replace(/\\/g, '/')}/` : '../../../data/kb/', import.meta.url);
const outDir = new URL('../public/wiki/', import.meta.url);

const SPACES = {
  onboarding: 'Онбординг', process: 'Процессы', it: 'IT и доступы', hr: 'HR', observability: 'Мониторинг',
  security: 'Безопасность', faq: 'FAQ', services: 'Сервисы',
};

const esc = s => String(s).replace(/[&<>"]/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'}[c]));
const inline = s => esc(s).replace(/\*\*(.+?)\*\*/g, '<b>$1</b>').replace(/`([^`]+)`/g, '<code>$1</code>')
  .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+|\/[^)\s]*)\)/g, '<a href="$2">$1</a>');
export const slug = s => s.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^\p{L}\p{N}_-]/gu, '');

function frontMatter(raw) {
  const text = raw.replace(/\r\n/g, '\n');
  if (!text.startsWith('---\n')) return [{}, text];
  const end = text.indexOf('\n---\n', 4);
  const meta = {};
  for (const line of text.slice(4, end).split('\n')) {
    const i = line.indexOf(':');
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return [meta, text.slice(end + 5)];
}

function body(md) {
  let html = '', list = false;
  const toc = [];
  for (const line of md.split('\n')) {
    const item = /^\s*(?:[-*•]|\d+\.)\s+(.*)$/.exec(line);
    if (item) { if (!list) { html += '<ul>'; list = true; } html += `<li>${inline(item[1])}</li>`; continue; }
    if (list) { html += '</ul>'; list = false; }
    if (line.startsWith('## ')) {
      const heading = line.slice(3).trim();
      toc.push(heading);
      html += `<h2 id="${esc(slug(heading))}">${inline(heading)}</h2>`;
    } else if (line.trim()) {
      html += `<p>${inline(line)}</p>`;
    }
  }
  return {html: html + (list ? '</ul>' : ''), toc};
}

const space = doc => (doc.url.match(/^https?:\/\/[^/]+\/([^/]+)/)?.[1]) || 'other';

const CSS = `
/* Оформление общее с демо-порталом заявок (apps/web/src/styles/portal.css): один «корпоративный» стиль */
:root{--bg:#eef1f6;--card:#fff;--ink:#1c2330;--muted:#5b6577;--line:#d9dfe9;--accent:#1e3a8a;--side:#fff;--top:#1e3a8a;--warn:#fff3d7}
*{box-sizing:border-box}html{scroll-padding-top:70px}
body{margin:0;font:15px/1.6 'Segoe UI',system-ui,-apple-system,sans-serif;background:var(--bg);color:var(--ink)}
.top{position:sticky;top:0;z-index:2;display:flex;align-items:center;gap:28px;min-height:56px;padding:0 28px;background:var(--top);color:#fff}
.top a{color:#fff;text-decoration:none}.brand{display:inline-flex;align-items:center;gap:10px;font-size:18px;font-weight:700}
.brand small{font-size:11px;font-weight:600;padding:2px 8px;border-radius:6px;background:#f5cd47;color:#3d3200}
.top nav{display:flex;gap:22px;margin-right:auto;font-size:15px}
.top nav a{color:#ffffffb3;padding:17px 0;border-bottom:3px solid transparent}.top nav a.is-on{color:#fff;border-bottom-color:#fff}
.wrap{display:grid;grid-template-columns:260px minmax(0,1fr);min-height:calc(100vh - 56px)}
aside{background:var(--side);border-right:1px solid var(--line);padding:16px 12px;font-size:14px}
aside h4{margin:14px 8px 4px;font-size:11px;text-transform:uppercase;letter-spacing:.6px;color:var(--muted)}
aside a{display:block;padding:4px 8px;border-radius:6px;color:var(--ink);text-decoration:none}aside a:hover{background:var(--bg)}
aside a.on{background:color-mix(in srgb,var(--accent) 12%,transparent);color:var(--accent);font-weight:600}
main{padding:28px 40px 60px;max-width:900px}
.crumbs{font-size:14px;color:var(--muted)}.crumbs a{color:var(--accent);text-decoration:none}
h1{font-size:30px;line-height:1.25;margin:6px 0 4px}.meta{font-size:13px;color:var(--muted);margin-bottom:16px}
.note{background:var(--warn);border-radius:8px;padding:8px 12px;font-size:13px;margin-bottom:18px;color:#8b5b00}
.toc{border-left:3px solid var(--line);padding:2px 12px;margin:0 0 18px;font-size:14px}.toc a{color:var(--accent);text-decoration:none;display:block}
article{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:22px 28px;box-shadow:0 2px 10px #1e3a8a0f}
article h2{font-size:20px;margin:26px 0 6px;padding-top:4px}article h2:first-child{margin-top:0}article p{margin:0 0 10px}article ul,article ol{margin:0 0 10px;padding-left:22px}
article code{background:var(--bg);border-radius:4px;padding:0 5px;font-size:13px}
article a{color:var(--accent)}
:target{background:color-mix(in srgb,var(--accent) 10%,transparent);border-radius:4px}
.cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:12px}
.cards a{display:block;background:var(--card);border:1px solid var(--line);border-radius:12px;padding:12px 14px;color:var(--ink);text-decoration:none}
.cards a:hover{border-color:var(--accent)}.cards small{color:var(--muted)}
@media (max-width:760px){.top{flex-wrap:wrap;gap:6px 18px;padding:8px 16px}.top nav{order:3;width:100%}.wrap{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid var(--line)}main{padding:20px 16px 40px}article{padding:16px}}
`;

function page({title, docs, current, content}) {
  const groups = {};
  for (const d of docs) (groups[space(d)] ||= []).push(d);
  const nav = Object.entries(groups).map(([key, items]) => `<h4>${esc(SPACES[key] || key)}</h4>${
    items.map(d => `<a href="${d.id}.html"${d.id === current ? ' class="on"' : ''}>${esc(d.title)}</a>`).join('')}`).join('');
  return `<!doctype html>
<!-- Демо-вики AI-buddy: синтетические документы базы знаний (data/kb). Не регламенты Авито. -->
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>${esc(title)} · База знаний (демо)</title><style>${CSS}</style></head><body>
<header class="top"><a class="brand" href="index.html"><span aria-hidden="true">▦</span> Портал сотрудника <small>демо</small></a><nav aria-label="Разделы портала"><a href="/portal/requests">Заявки</a><a class="is-on" href="index.html">База знаний</a></nav></header>
<div class="wrap"><aside><a href="index.html"${current ? '' : ' class="on"'}>Все документы</a>${nav}</aside><main>${content}</main></div>
</body></html>`;
}

const BUDDY = `## Что это
Бади — AI-наставник стажёра. Он живёт в правом нижнем углу каждой страницы: на сайте адаптации и, если установить расширение, на любом рабочем сайте.
## Что он умеет
- Отвечает на рабочие вопросы по базе знаний со ссылками на документы и честно говорит о противоречиях между ними.
- Помогает оформить заявку на отгул, отпуск или доступ: объясняет, как это делается, и предлагает составить заявку вместе. Заявка создаётся только после вашего подтверждения.
- Передаёт вопрос наставнику, если ответа в базе нет.
- Сам подсказывает: когда задача идёт дольше плана, когда заявку согласовали и когда наставник ответил.
- **⤢ / ⤡** — компактный и большой режим окна. В большом режиме есть вкладки: вопросы наставнику, заявки, заметки и календарь.
## Как поставить расширение Chrome
- Откройте страницу установки: [Установка расширения](/install-extension.html) (в демо это заглушка).
- Вручную: откройте \`chrome://extensions\`, включите «Режим разработчика» и нажмите «Загрузить распакованное расширение».
- Выберите папку \`apps/extension/dist\` (сборка описана в README проекта).
- Закрепите значок AI-buddy на панели браузера и обновите открытые вкладки. Вход общий с сайтом.
## Попробовать на демо-страницах
- [Вики команды](/demo/wiki.html)
- [Дашборд](/demo/grafana.html)
- [Доска задач](/demo/jira.html)
`;

const files = (await readdir(kbDir)).filter(f => f.endsWith('.md')).sort();
const docs = [];
for (const file of files) {
  const [meta, md] = frontMatter(await readFile(new URL(file, kbDir), 'utf8'));
  if (meta.id) docs.push({...meta, md});
}
docs.push({id: 'buddy', title: 'Помощник Бади и расширение Chrome', url: 'https://wiki.example/services/buddy', updated: '2026-11-15', md: BUDDY});
docs.sort((a, b) => space(a).localeCompare(space(b)) || a.title.localeCompare(b.title, 'ru'));

await rm(outDir, {recursive: true, force: true});
await mkdir(outDir, {recursive: true});
for (const d of docs) {
  const {html, toc} = body(d.md);
  const content = `<div class="crumbs"><a href="index.html">База знаний</a> / ${esc(SPACES[space(d)] || space(d))}</div>
<h1>${esc(d.title)}</h1><div class="meta">Обновлено ${esc(d.updated || '—')}</div>
<div class="note">Демо-документ для прототипа AI-buddy. Не является регламентом Авито или другой реальной компании.</div>
${toc.length > 1 ? `<nav class="toc">${toc.map(h => `<a href="#${esc(slug(h))}">${esc(h)}</a>`).join('')}</nav>` : ''}
<article>${html}</article>`;
  await writeFile(new URL(`${d.id}.html`, outDir), page({title: d.title, docs, current: d.id, content}));
}
const index = `<h1>База знаний</h1><div class="meta">${docs.length} документов</div>
<div class="note">Демо-вики: синтетическая база знаний, на которую опирается Бади. Не является регламентами Авито.</div>
<div class="cards">${docs.map(d => `<a href="${d.id}.html"><b>${esc(d.title)}</b><br><small>${esc(SPACES[space(d)] || space(d))} · ${esc(d.updated || '')}</small></a>`).join('')}</div>`;
await writeFile(new URL('index.html', outDir), page({title: 'Все документы', docs, current: '', content: index}));
console.log(`wiki: ${docs.length} documents -> apps/web/public/wiki`);
