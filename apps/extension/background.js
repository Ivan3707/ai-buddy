// No tab access or background network calls. MV3 lifecycle hook only.
chrome.runtime.onInstalled.addListener(() => {
  console.info('AI-buddy demo installed; start the local backend to chat.');
});
