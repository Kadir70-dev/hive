export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
 const response = await fetch(`/api/backend${path}`, {
   ...init, credentials: 'same-origin', cache: 'no-store',
   headers: { 'Content-Type': 'application/json', ...(init.headers || {}) },
 });
 const result = await response.json().catch(() => ({}));
 if (!response.ok) throw new Error(result.error || 'Request failed');
 return result as T;
}
export function send<T>(path: string, value: unknown, method = 'POST') {
 return api<T>(path, { method, body: JSON.stringify(value) });
}
