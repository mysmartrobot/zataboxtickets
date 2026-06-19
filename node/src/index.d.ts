// Type definitions for @zatabox/node.
// The per-namespace method types live in ./resources.generated.d.ts.

import type { ResourceNamespaces, WebhooksNamespace, MediaNamespace } from './resources.generated';

export const VERSION: string;

export interface ClientOptions {
  /** vt_live_… / vt_test_… API key. Test keys auto-route to the sandbox. */
  apiKey?: string;
  /** Portal JWT access token or vt_mcp_… token. */
  bearerToken?: string;
  /** Explicit base URL override (wins over key-prefix routing). */
  baseUrl?: string;
  /** fetch implementation (defaults to global fetch; Node 18+). */
  fetch?: typeof fetch;
  /** Per-request timeout in ms (default 30000). */
  timeoutMs?: number;
  /** Transient-failure retries (default 2). */
  maxRetries?: number;
  /** Override the User-Agent header. */
  userAgent?: string;
}

export interface RequestOptions {
  /** Idempotency-Key for writes (auto-generated UUIDv4 if omitted). */
  idempotencyKey?: string;
  /** Extra/override query parameters. */
  query?: Record<string, any>;
  /** Extra/override request headers. */
  headers?: Record<string, string>;
  /** Return raw bytes instead of a parsed envelope. */
  raw?: boolean;
}

export interface BinaryResponse {
  data: Buffer;
  contentType: string | null;
  filename: string | null;
}

export interface UploadOptions {
  filename?: string;
  contentType?: string;
  /** Form field name for the file (default "file"). */
  field?: string;
  /** Extra form fields. */
  fields?: Record<string, string | number>;
}

export class ZataboxError extends Error {
  name: 'ZataboxError';
  code: string;
  status: number;
  requestId: string | null;
  details?: any;
}

/** Webhook namespace plus the inbound signature verifier. */
export interface WebhooksNamespaceExt extends WebhooksNamespace {
  /** Verify an inbound webhook signature; returns the parsed event or throws. */
  verify(payload: string | object, signatureHeader: string, secret: string, toleranceSec?: number): any;
}

/** Media namespace plus multipart upload. */
export interface MediaNamespaceExt extends MediaNamespace {
  upload(file: Buffer | Uint8Array | Blob, opts?: UploadOptions): Promise<any>;
}

export type Client = ZataboxClient;

export class ZataboxClient implements Omit<ResourceNamespaces, 'webhooks' | 'media'> {
  constructor(options: ClientOptions);

  apiKey: string | null;
  baseUrl: string;
  timeoutMs: number;
  maxRetries: number;
  readonly token: string | null;

  setBearerToken(token: string): this;

  request(
    method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE',
    path: string,
    opts?: { query?: Record<string, any>; body?: any; idempotencyKey?: string; headers?: Record<string, string>; raw?: boolean }
  ): Promise<any>;

  paginate(listFn: (query?: Record<string, any>) => Promise<any>, query?: Record<string, any>): AsyncGenerator<any>;

  webhooks: WebhooksNamespaceExt;
  media: MediaNamespaceExt;

  // Generated namespaces.
  auth: ResourceNamespaces['auth'];
  users: ResourceNamespaces['users'];
  savedSearches: ResourceNamespaces['savedSearches'];
  dataExport: ResourceNamespaces['dataExport'];
  events: ResourceNamespaces['events'];
  tickets: ResourceNamespaces['tickets'];
  orders: ResourceNamespaces['orders'];
  payments: ResourceNamespaces['payments'];
  checkin: ResourceNamespaces['checkin'];
  scan: ResourceNamespaces['scan'];
  search: ResourceNamespaces['search'];
  organizer: ResourceNamespaces['organizer'];
  wallets: ResourceNamespaces['wallets'];
  scannerTokens: ResourceNamespaces['scannerTokens'];
  integrations: ResourceNamespaces['integrations'];
  eventCustomization: ResourceNamespaces['eventCustomization'];
  growth: ResourceNamespaces['growth'];
  publicEvents: ResourceNamespaces['publicEvents'];
  site: ResourceNamespaces['site'];
  community: ResourceNamespaces['community'];
  track: ResourceNamespaces['track'];
  whiteLabel: ResourceNamespaces['whiteLabel'];
  support: ResourceNamespaces['support'];
  util: ResourceNamespaces['util'];
}
