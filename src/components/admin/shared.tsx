'use client';
import { useState } from 'react';
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';

export type Paged<T> = { items: T[]; page: number; page_size: number; total: number; has_more: boolean };

/** Paginated admin list with search; keeps the previous page visible while loading. */
export function usePaged<T>(key: string, path: string, params: Record<string, string | number>) {
  const qs = new URLSearchParams(Object.entries(params).filter(([, v]) => v !== '' && v !== undefined).map(([k, v]) => [k, String(v)])).toString();
  return useQuery({ queryKey: ['admin', key, qs], queryFn: () => api<Paged<T>>(`${path}?${qs}`), placeholderData: keepPreviousData });
}

/** Runs an admin mutation, refreshes admin data and reports the outcome. */
export function useAdminAction() {
  const qc = useQueryClient();
  const [message, setMessage] = useState<{ text: string; tone: 'ok' | 'err' }>({ text: '', tone: 'ok' });
  const [busy, setBusy] = useState(false);
  async function run<T = unknown>(path: string, method: string, payload?: unknown, success: string | ((out: T) => string) = 'Saved'): Promise<T | null> {
    setBusy(true);
    try {
      const out = await api<T>(path, { method, body: payload === undefined ? undefined : JSON.stringify(payload) });
      await qc.invalidateQueries({ queryKey: ['admin'] });
      setMessage({ text: typeof success === 'function' ? success(out) : success, tone: 'ok' });
      return out;
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Request failed', tone: 'err' });
      return null;
    } finally { setBusy(false); }
  }
  return { run, busy, message };
}

export function SearchBox({ value, onChange, label }: { value: string; onChange: (v: string) => void; label: string }) {
  return <label>{label}<input type="search" value={value} onChange={(e) => onChange(e.target.value)} maxLength={100} /></label>;
}

/** https-only URL check mirroring the API. */
export function isHttpsURL(v: string): boolean {
  if (!v) return true;
  try { const u = new URL(v); return u.protocol === 'https:' && !!u.host && !u.username; } catch { return false; }
}
