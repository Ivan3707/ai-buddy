import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';

// @buddy: the Бади widget and its backend gateway are shared with the Chrome extension (apps/extension).
const extension = fileURLToPath(new URL('../extension', import.meta.url));
export default defineConfig({
  resolve: {alias: {'@buddy': extension}},
  server: {proxy: {'/api': 'http://127.0.0.1:8080'}, fs: {allow: ['..', '../../packages']}},
});
