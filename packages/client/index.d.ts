export type ChatResponse = { answer: string; sources: {id: string; title: string; url: string}[]; mode: 'mock' };
export function chat(baseUrl: string, message: string): Promise<ChatResponse>;
