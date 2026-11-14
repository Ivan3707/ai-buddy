// Types for the shared Бади widget and gateway (plain JS in apps/extension, aliased as @buddy in vite.config.ts).

declare module '@buddy/buddy/widget.js' {}

declare module '@buddy/lib/gateway.js' {
  export type GatewayMessage = { op: string; [key: string]: unknown };
  export function createGateway(options: {
    baseUrl: string;
    auth: import('@buddy/lib/auth.js').Auth;
    hooks?: { onLogin?(): void; onLogout?(): void };
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
  refresh(): Promise<void>;
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
      portalUrl?(kind: string, id: string): string;
    }): BuddyApi;
  };
}

declare module '@buddy/lib/auth.js' {
  type Pair = { access_token: string; refresh_token: string; expires_in: number };
  export type Auth = {
    read(): Promise<{ access: string; refresh: string; expiresAt: number; sub: string; loginAt: number } | null>;
    info(): Promise<{ sub: string; loginAt: number; signedOutAt: number }>;
    token(options?: { force?: boolean; stale?: string }): Promise<string>;
    adopt(pair: Pair, options?: { loginAt?: number }): Promise<void>;
    login(email: string, password: string): Promise<unknown>;
    link(): Promise<Pair | null>;
    logout(): Promise<void>;
    clear(): Promise<void>;
    onChange(callback: () => void): () => void;
  };
  export function createAuth(options: {
    baseUrl: string;
    storage: { get(key: string): Promise<unknown>; set(key: string, value: unknown): Promise<void>; remove(key: string): Promise<void> };
    client: Record<string, (...args: any[]) => Promise<any>>;
    lock?: (name: string, fn: () => Promise<any>) => Promise<any>;
    watch?: (onChange: () => void) => () => void;
  }): Auth;
}
