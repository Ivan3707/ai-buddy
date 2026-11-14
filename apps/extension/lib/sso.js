// Single sign-on between the web app and the extension: who adopts whose session.
//
// Each side has {sub, loginAt, signedOutAt} (sub = user id of its session, loginAt = when the user last signed in
// on that side, signedOutAt = when the user last signed out there). The extension decides on every handshake:
//   'none'   nothing to do
//   'push'   the site is the newer / only signed-in side: it links a session for the extension
//   'pull'   the extension is the newer / only signed-in side: it links a session for the site
//   'logout' the user signed out in the extension after the site's sign-in: the site signs out as well
export function decideSync(site, ext) {
  const siteIn = Boolean(site?.sub), extIn = Boolean(ext?.sub);
  if (!siteIn && !extIn) return 'none';
  if (siteIn && !extIn) return (ext?.signedOutAt || 0) > (site.loginAt || 0) ? 'logout' : 'push';
  if (!siteIn && extIn) return (site?.signedOutAt || 0) > (ext.loginAt || 0) ? 'ext-logout' : 'pull';
  if (site.sub === ext.sub) return 'none'; // the same user on both sides: each keeps its own session
  return (site.loginAt || 0) > (ext.loginAt || 0) ? 'push' : 'pull';
}
