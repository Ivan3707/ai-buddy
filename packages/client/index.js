export async function chat(baseUrl, message) {
  const response = await fetch(`${baseUrl}/api/chat`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }), signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new Error(`Backend: HTTP ${response.status}`);
  return response.json();
}
