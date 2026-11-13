// MV3 service worker: no network calls and no tab access here.
// Clicking the toolbar icon opens the side panel; it stays open while the user browses
// (a popup would close as soon as it loses focus).
chrome.sidePanel
  .setPanelBehavior({openPanelOnActionClick: true})
  .catch(error => console.error('AI-buddy: cannot enable side panel', error));

chrome.runtime.onInstalled.addListener(() => {
  console.info('AI-buddy demo installed; start the local backend to chat.');
});
