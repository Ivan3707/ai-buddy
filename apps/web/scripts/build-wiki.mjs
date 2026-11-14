// Builds the demo corporate wiki (static HTML) from the synthetic knowledge base data/kb/*.md.
// The bot cites these pages as sources: "Открыть документ полностью" leads here.
// Output: apps/web/public/wiki/<id>.html + index.html (generated, not in git). Synthetic data, labelled as demo.
import { mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';

const kbDir = new URL(process.env.KB_DIR ? `file:///${process.env.KB_DIR.replace(/\\/g, '/')}/` : '../../../data/kb/', import.meta.url);
const outDir = new URL('../public/wiki/', import.meta.url);

const SPACES = {
  onboarding: 'Онбординг', process: 'Процессы', it: 'IT и доступы', hr: 'HR', observability: 'Мониторинг',
  security: 'Безопасность', faq: 'FAQ',
};

const esc = s => String(s).replace(/[&<>"]/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'}[c]));
const inline = s => esc(s).replace(/\*\*(.+?)\*\*/g, '<b>$1</b>').replace(/`([^`]+)`/g, '<code>$1</code>');
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
:root{--bg:#f4f5f7;--card:#fff;--ink:#172b4d;--muted:#5e6c84;--line:#dfe1e6;--accent:#0c66e4;--side:#fafbfc;--top:#1d2125;--warn:#fff7d6}
@media (prefers-color-scheme:dark){:root{--bg:#161a1d;--card:#1d2125;--ink:#dee4ea;--muted:#9fadbc;--line:#2c333a;--accent:#579dff;--side:#1a1e22;--top:#101214;--warn:#3d3200}}
*{box-sizing:border-box}html{scroll-padding-top:70px}
body{margin:0;font:15px/1.6 -apple-system,"Segoe UI",system-ui,sans-serif;background:var(--bg);color:var(--ink)}
.top{position:sticky;top:0;z-index:2;display:flex;align-items:center;gap:16px;height:52px;padding:0 20px;background:var(--top);color:#fff}
.top a{color:#fff;text-decoration:none}.logo{font-weight:700;letter-spacing:.3px}.logo span{opacity:.6;font-weight:400}
.demo{margin-left:auto;font-size:12px;background:#f5cd47;color:#3d3200;border-radius:4px;padding:2px 8px;font-weight:600}
.wrap{display:grid;grid-template-columns:260px minmax(0,1fr);min-height:calc(100vh - 52px)}
aside{background:var(--side);border-right:1px solid var(--line);padding:16px 12px;font-size:14px}
aside h4{margin:14px 8px 4px;font-size:11px;text-transform:uppercase;letter-spacing:.6px;color:var(--muted)}
aside a{display:block;padding:4px 8px;border-radius:4px;color:var(--ink);text-decoration:none}aside a:hover{background:var(--line)}
aside a.on{background:color-mix(in srgb,var(--accent) 14%,transparent);color:var(--accent);font-weight:600}
main{padding:28px 40px 60px;max-width:900px}
.crumbs{font-size:13px;color:var(--muted)}.crumbs a{color:var(--accent);text-decoration:none}
h1{font-size:28px;line-height:1.25;margin:6px 0 4px}.meta{font-size:13px;color:var(--muted);margin-bottom:16px}
.note{background:var(--warn);border-radius:6px;padding:8px 12px;font-size:13px;margin-bottom:18px}
.toc{border-left:3px solid var(--line);padding:2px 12px;margin:0 0 18px;font-size:14px}.toc a{color:var(--accent);text-decoration:none;display:block}
article h2{font-size:20px;margin:26px 0 6px;padding-top:4px}article p{margin:0 0 10px}article ul{margin:0 0 10px;padding-left:22px}
article code{background:var(--line);border-radius:3px;padding:0 4px;font-size:13px}
:target{background:color-mix(in srgb,var(--accent) 10%,transparent);border-radius:4px}
.cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:12px}
.cards a{display:block;background:var(--card);border:1px solid var(--line);border-radius:8px;padding:12px 14px;color:var(--ink);text-decoration:none}
.cards a:hover{border-color:var(--accent)}.cards small{color:var(--muted)}
@media (max-width:760px){.wrap{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid var(--line)}main{padding:20px 16px 40px}}
`;

function page({title, docs, current, content}) {
  const groups = {};
  for (const d of docs) (groups[space(d)] ||= []).push(d);
  const nav = Object.entries(groups).map(([key, items]) => `<h4>${esc(SPACES[key] || key)}</h4>${
    items.map(d => `<a href="${d.id}.html"${d.id === current ? ' class="on"' : ''}>${esc(d.title)}</a>`).join('')}`).join('');
  return `<!doctype html>
<!-- Демо-вики AI-buddy: синтетические документы базы знаний (data/kb). Не регламенты Авито. -->
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>${esc(title)} · Вики компании (демо)</title><style>${CSS}</style></head><body>
<header class="top"><a class="logo" href="index.html">Вики компании <span>· Confluence-демо</span></a><span class="demo">ДЕМО · данные синтетические</span></header>
<div class="wrap"><aside><a href="index.html"${current ? '' : ' class="on"'}>Все документы</a>${nav}</aside><main>${content}</main></div>
</body></html>`;
}

const files = (await readdir(kbDir)).filter(f => f.endsWith('.md')).sort();
const docs = [];
for (const file of files) {
  const [meta, md] = frontMatter(await readFile(new URL(file, kbDir), 'utf8'));
  if (meta.id) docs.push({...meta, md});
}
docs.sort((a, b) => space(a).localeCompare(space(b)) || a.title.localeCompare(b.title, 'ru'));

await rm(outDir, {recursive: true, force: true});
await mkdir(outDir, {recursive: true});
for (const d of docs) {
  const {html, toc} = body(d.md);
  const content = `<div class="crumbs"><a href="index.html">Вики</a> / ${esc(SPACES[space(d)] || space(d))}</div>
<h1>${esc(d.title)}</h1><div class="meta">Обновлено ${esc(d.updated || '—')}</div>
<div class="note">Демо-документ для прототипа AI-buddy. Не является регламентом Авито или другой реальной компании.</div>
${toc.length > 1 ? `<nav class="toc">${toc.map(h => `<a href="#${esc(slug(h))}">${esc(h)}</a>`).join('')}</nav>` : ''}
<article>${html}</article>`;
  await writeFile(new URL(`${d.id}.html`, outDir), page({title: d.title, docs, current: d.id, content}));
}
const index = `<h1>Вики компании</h1><div class="meta">${docs.length} документов</div>
<div class="note">Демо-вики: синтетическая база знаний, на которую опирается Бади. Не является регламентами Авито.</div>
<div class="cards">${docs.map(d => `<a href="${d.id}.html"><b>${esc(d.title)}</b><br><small>${esc(SPACES[space(d)] || space(d))} · ${esc(d.updated || '')}</small></a>`).join('')}</div>`;
await writeFile(new URL('index.html', outDir), page({title: 'Все документы', docs, current: '', content: index}));
console.log(`wiki: ${docs.length} documents -> apps/web/public/wiki`);
