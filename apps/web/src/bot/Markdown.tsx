import { Fragment, type ReactNode } from 'react';
import type { Source } from './api';

// Minimal safe markdown for bot answers: paragraphs, "-"/"•"/"1." lists, **bold**, `code` and [n] citations.
// Builds React elements only (no innerHTML), so model output cannot inject markup.

function inline(text: string, sources: Source[] | undefined, key: string): ReactNode[] {
  const out: ReactNode[] = [];
  const pattern = /\*\*([^*]+)\*\*|`([^`]+)`|\[(\d+(?:\s*,\s*\d+)*)(?:,[^\]]*)?\]/g;
  let last = 0; let match: RegExpExecArray | null; let i = 0;
  while ((match = pattern.exec(text))) {
    if (match.index > last) out.push(text.slice(last, match.index));
    if (match[1]) out.push(<b key={`${key}b${i}`}>{match[1]}</b>);
    else if (match[2]) out.push(<code key={`${key}c${i}`}>{match[2]}</code>);
    else {
      const numbers = match[3].split(',').map(n => Number(n.trim()));
      out.push(<sup key={`${key}s${i}`} className="bot-cite">{numbers.map((n, j) => {
        const source = sources?.[n - 1];
        return <Fragment key={j}>{j > 0 && ','}{source && /^https?:\/\//.test(source.url)
          ? <a href={source.url} target="_blank" rel="noreferrer" title={source.title}>{n}</a> : n}</Fragment>;
      })}</sup>);
    }
    last = match.index + match[0].length; i++;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

export function Markdown({ text, sources }: { text: string; sources?: Source[] }) {
  const blocks: ReactNode[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;
  const flush = () => {
    if (!list) return;
    const items = list.items.map((item, i) => <li key={i}>{inline(item, sources, `l${blocks.length}-${i}`)}</li>);
    blocks.push(list.ordered ? <ol key={blocks.length}>{items}</ol> : <ul key={blocks.length}>{items}</ul>);
    list = null;
  };
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    const bullet = /^[-•*]\s+(.*)$/.exec(line); const numbered = /^\d+[.)]\s+(.*)$/.exec(line);
    if (bullet || numbered) {
      const ordered = Boolean(numbered);
      if (list && list.ordered !== ordered) flush();
      list = list ?? { ordered, items: [] };
      list.items.push((bullet || numbered)![1]);
      continue;
    }
    flush();
    if (!line) continue;
    const heading = /^#{1,4}\s+(.*)$/.exec(line);
    blocks.push(heading ? <p key={blocks.length}><b>{heading[1]}</b></p> : <p key={blocks.length}>{inline(line, sources, `p${blocks.length}`)}</p>);
  }
  flush();
  return <div className="bot-md">{blocks}</div>;
}
