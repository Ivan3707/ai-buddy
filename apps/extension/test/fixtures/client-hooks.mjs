// Module resolve hook for background.test.js: background.js imports './client.js', which build.mjs copies from
// packages/client into dist/. In the source tree it does not exist, so it resolves to the scripted fake client.
export async function resolve(specifier, context, next) {
  if (specifier === './client.js' && context.parentURL?.endsWith('/apps/extension/background.js')) {
    return {url: new URL('./fake-client.js', import.meta.url).href, shortCircuit: true};
  }
  return next(specifier, context);
}
