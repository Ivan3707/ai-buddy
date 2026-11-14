// Types for the shared Бади widget and gateway (plain JS in apps/extension, aliased as @buddy in vite.config.ts).

declare module '@buddy/buddy/widget.js' {}

declare module '@buddy/lib/gateway.js' {
  export type GatewayMessage = { op: string; [key: string]: unknown };
  export function createGateway(options: {
    baseUrl: string;
    tokens: { get(): Promise<string>; set?(token: string): Promise<void> };
    store: { get(key: string): Promise<unknown>; set(key: string, value: unknown): Promise<void>; remove(key: string): Promise<void> };
    client: { getMe(baseUrl: string, token: string): Promise<unknown> };
  }): {
    call(message: GatewayMessage): Promise<unknown>;
    stream(payload: GatewayMessage, send: (message: unknown) => void): () => void;
  };
}

type BuddyButton = { label: string; onClick?: () => void; ghost?: boolean };
type BuddySayOptions = {
  detail?: string; button?: string; onClick?: () => void; buttons?: BuddyButton[];
  ms?: number; mood?: 'waving' | 'happy' | 'curious' | 'sad'; queue?: boolean;
};
interface BuddyApi {
  host: HTMLElement;
  show(): void;
  remove(): void;
  isShown(): boolean;
  isOpen(): boolean;
  open(): Promise<void>;
  close(): void;
  toggle(): void;
  ask(text: string): Promise<void>;
  say(text: string, options?: BuddySayOptions): Promise<void>;
}
interface Window {
  AIBuddyWidget?: {
    mount(env: {
      call(message: { op: string; [key: string]: unknown }): Promise<unknown>;
      stream(payload: { op: string; [key: string]: unknown }, onMessage: (message: unknown) => void): () => void;
      storage: { get(key: string): Promise<unknown>; set(key: string, value: unknown): Promise<void> };
      features?: { login?: boolean; pageContext?: boolean; hide?: boolean; requests?: boolean };
      onHide?(): void;
    }): BuddyApi;
  };
}
