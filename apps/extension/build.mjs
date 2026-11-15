import { cp, copyFile, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises';

// BACKEND_ORIGIN: where /api/* lives. The web app proxies /api to the backend, so the site origin works too.
// SITE_ORIGIN: the AI-buddy web app (single sign-on, request portal links). Both are plain origins without a path.
const plainOrigin = (name, fallback) => {
  const value = process.env[name] || fallback;
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.origin !== value) {
    throw new Error(`${name} must be a plain http(s)://host[:port] origin, got ${value}`);
  }
  return url;
};
const url = plainOrigin('BACKEND_ORIGIN', 'http://127.0.0.1:8080');
const origin = url.origin;
const site = plainOrigin('SITE_ORIGIN', 'http://127.0.0.1:5173');
// the local site answers on both 127.0.0.1 and localhost
const siteOrigins = site.hostname === '127.0.0.1' ? [site.origin, `${site.protocol}//localhost${site.port ? `:${site.port}` : ''}`] : [site.origin];
const escape = text => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const siteHosts = siteOrigins.map(o => escape(new URL(o).host)).join('|');

const source = new URL('./', import.meta.url);
const dist = new URL('./dist/', import.meta.url);
await rm(dist, {recursive: true, force: true});
await mkdir(dist, {recursive: true});

const files = [
  'manifest.json', 'background.js', 'config.js',
];
for (const file of files) {
  await copyFile(new URL(file, source), new URL(file, dist));
}
await cp(new URL('./buddy/', source), new URL('./buddy/', dist), {recursive: true});
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
// The web app origin is baked into the SSO allow-list (background) and the on-site detection (content script).
const rewrite = async (file, pairs) => {
  const output = new URL(file, dist);
  let code = await readFile(output, 'utf8');
  for (const [from, to] of pairs) {
    if (!code.includes(from)) throw new Error(`${file}: expected ${from}`);
    code = code.replace(from, to);
  }
  await writeFile(output, code);
};
await rewrite('background.js', [["const WEB_ORIGINS = ['http://127.0.0.1:5173', 'http://localhost:5173'];", `const WEB_ORIGINS = ${JSON.stringify(siteOrigins).replaceAll('"', "'")};`]]);
await rewrite('buddy/content.js', [
  ["const SITE_ORIGIN = 'http://127.0.0.1:5173';", `const SITE_ORIGIN = '${site.origin}';`],
  ['const WEB_APP = /^(127\\.0\\.0\\.1|localhost):5173$/;', `const WEB_APP = /^(${siteHosts})$/;`],
]);
// Sanity check of the output: valid manifest, referenced files and relative imports exist.
const exists = file => stat(new URL(file, dist)).then(() => true, () => false);
const manifest = JSON.parse(await readFile(new URL('manifest.json', dist), 'utf8'));
const referenced = [
  manifest.background.service_worker,
  'buddy/bridge.js',
  'buddy/widget.js',
  'buddy/content.js',
];
const modules = ['background.js'];
for (let i = 0; i < modules.length; i += 1) {
  const code = await readFile(new URL(modules[i], dist), 'utf8');
  for (const [, target] of code.matchAll(/^import[^'"]*['"](\.[^'"]+)['"]/gm)) {
    const resolved = new URL(target, new URL(modules[i], dist)).href.slice(dist.href.length);
    if (!modules.includes(resolved)) modules.push(resolved);
  }
}
const missing = [];
for (const file of new Set([...referenced, ...modules])) {
  if (!(await exists(file))) missing.push(file);
}
if (missing.length) throw new Error(`Build output is missing: ${missing.join(', ')}`);

console.log(`AI-buddy extension built into apps/extension/dist (backend ${origin}, site ${site.origin})`);
