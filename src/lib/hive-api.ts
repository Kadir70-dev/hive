/** Error from the Go API; `code` is the machine-readable reason (e.g. "mfa_required"). */
export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string, readonly retryAfter?: number) {
    super(message);
    this.name = 'ApiError';
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
 const response = await fetch(`/api/backend${path}`, {
   ...init, credentials: 'same-origin', cache: 'no-store',
   headers: { 'Content-Type': 'application/json', ...(init.headers || {}) },
 });
 const result = await response.json().catch(() => ({}));
 if (!response.ok) {
   const retry = Number(response.headers.get('retry-after')) || undefined;
   throw new ApiError(result.error || 'Request failed', response.status, result.code, retry);
 }
 return result as T;
}
export function send<T>(path: string, value: unknown, method = 'POST') {
 return api<T>(path, { method, body: JSON.stringify(value) });
}
/** Reads a one-time token from the URL fragment (#token=…), which never reaches servers or logs. */
export function fragmentToken(hash: string): string {
 return new URLSearchParams(hash.replace(/^#/, '')).get('token') ?? '';
}
