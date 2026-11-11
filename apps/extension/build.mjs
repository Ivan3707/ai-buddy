import { mkdir, copyFile, readFile, writeFile } from 'node:fs/promises';
const origin = process.env.BACKEND_ORIGIN || 'http://127.0.0.1:8080';
const url = new URL(origin);
if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.origin !== origin) {
  throw new Error('BACKEND_ORIGIN must be a plain http://127.0.0.1:<port> origin');
}
await mkdir(new URL('./dist/', import.meta.url), {recursive:true});
for (const file of ['manifest.json', 'popup.html', 'popup.js', 'popup-session.js', 'popup.css', 'background.js']) {
  await copyFile(new URL(file, import.meta.url), new URL(`dist/${file}`, import.meta.url));
}
await copyFile(new URL('../../packages/client/index.js', import.meta.url), new URL('dist/client.js', import.meta.url));
for (const file of ['manifest.json', 'popup.js']) {
  const output = new URL(`dist/${file}`, import.meta.url);
  await writeFile(output, (await readFile(output, 'utf8')).replaceAll('http://127.0.0.1:8080', origin));
}
