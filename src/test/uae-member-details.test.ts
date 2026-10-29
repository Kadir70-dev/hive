import { describe, expect, it } from 'vitest';
import { AREAS } from '@/lib/hive-i18n';
import { normalizeUAEPhone, isAbuDhabiArea } from '@/lib/uae-member-details';
import { readFileSync } from 'node:fs';

describe('UAE-only member contact details', () => {
  it('requires phone and normalizes local/international UAE mobiles', () => {
    expect(normalizeUAEPhone('')).toBeNull();
    expect(normalizeUAEPhone('  ')).toBeNull();
    expect(normalizeUAEPhone('050 123 4567')).toBe('+971501234567');
    expect(normalizeUAEPhone('+971 50 123 4567')).toBe('+971501234567');
    expect(normalizeUAEPhone('00971 50 123 4567')).toBe('+971501234567');
  });
  it('rejects foreign numbers, landlines and malformed values', () => {
    for (const phone of ['+91 9876543210', '+1 202 555 0123', '+971 2 123 4567', '987654', 'call me', '1234567890']) {
      expect(normalizeUAEPhone(phone)).toBeNull();
    }
  });
  it('offers only Abu Dhabi areas and requires selection at signup and profile', () => {
    expect(isAbuDhabiArea('Al Reem Island')).toBe(true);
    expect(isAbuDhabiArea('Other')).toBe(true);
    expect(isAbuDhabiArea('Dubai')).toBe(false);
    expect(isAbuDhabiArea('')).toBe(false);
    expect(AREAS).toContain('Khalifa City');
    const signup = readFileSync('src/components/screens/AuthScreen.tsx', 'utf8');
    const profile = readFileSync('src/components/screens/ProfileScreen.tsx', 'utf8');
    expect(signup).toContain('<select name="area" required');
    expect(signup).toContain('isAbuDhabiArea(area)');
    expect(signup).toContain('normalizeUAEPhone(');
    expect(signup).toContain('name="phone" type="tel" inputMode="tel" required');
    expect(profile).toContain('<select required value={area}');
    expect(profile).toContain('normalizeUAEPhone(phone)');
    expect(profile).toContain('type="tel" inputMode="tel" required');
    expect(profile).toContain('isAbuDhabiArea(area)');
  });
});
