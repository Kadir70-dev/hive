import { AREAS } from '@/lib/hive-i18n';

/** Parses a UAE mobile to +971 E.164; blank returns null because phone is required. */
export function normalizeUAEPhone(value: string): string | null {
  const raw = value.trim();
  if (!raw) return null;
  if (!/^[+0-9\s()\-]+$/.test(raw)) return null;
  const compact = raw.replace(/[\s()\-]/g, '');
  const international = compact.startsWith('00971') ? '+' + compact.slice(2) : compact;
  const e164 = /^05\d{8}$/.test(international) ? '+971' + international.slice(1) : international;
  return /^\+9715\d{8}$/.test(e164) ? e164 : null;
}

export function isAbuDhabiArea(value: string): boolean {
  return (AREAS as readonly string[]).includes(value);
}
