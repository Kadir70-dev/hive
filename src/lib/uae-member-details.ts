import { AREAS } from '@/lib/hive-i18n';

/** Blank is allowed; entered phone numbers must be UAE mobiles. Stores +971 E.164. */
export function normalizeUAEPhone(value: string): string | null {
  const raw = value.trim();
  if (!raw) return '';
  if (!/^[+0-9\s()\-]+$/.test(raw)) return null;
  const compact = raw.replace(/[\s()\-]/g, '');
  const international = compact.startsWith('00971') ? '+' + compact.slice(2) : compact;
  const e164 = /^05\d{8}$/.test(international) ? '+971' + international.slice(1) : international;
  return /^\+9715\d{8}$/.test(e164) ? e164 : null;
}

export function isAbuDhabiArea(value: string): boolean {
  return (AREAS as readonly string[]).includes(value);
}
