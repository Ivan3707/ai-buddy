import { cp, copyFile, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises';

const origin = process.env.BACKEND_ORIGIN || 'http://127.0.0.1:8080';
const url = new URL(origin);
if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.origin !== origin) {
  throw new Error('BACKEND_ORIGIN must be a plain http://127.0.0.1:<port> origin');
}

const source = new URL('./', import.meta.url);
const dist = new URL('./dist/', import.meta.url);
await rm(dist, {recursive: true, force: true});
await mkdir(dist, {recursive: true});

const files = [
  'manifest.json', 'background.js', 'config.js',
  'sidepanel.html', 'sidepanel.css', 'sidepanel.js', 'popup-session.js',
];
for (const file of files) {
  await copyFile(new URL(file, source), new URL(file, dist));
}
const lib = new URL('./lib/', source);
if (await stat(lib).then(() => true, () => false)) {
  await cp(lib, new URL('./lib/', dist), {recursive: true});
}
await copyFile(new URL('../../packages/client/index.js', source), new URL('client.js', dist));

// The backend origin is baked into the CSP, host_permissions and the runtime config.
for (const file of ['manifest.json', 'config.js']) {
  const output = new URL(file, dist);
  await writeFile(output, (await readFile(output, 'utf8')).replaceAll('http://127.0.0.1:8080', origin));
}
console.log(`AI-buddy extension built into apps/extension/dist (backend ${origin})`);
