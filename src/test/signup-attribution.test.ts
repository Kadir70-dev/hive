import { describe, expect, it } from 'vitest';
import { parseCampaign, captureCampaign, savedCampaign } from '@/lib/campaign-attribution';

describe('first-party campaign attribution', () => {
  it('accepts only allowlisted UTM values, not email or raw referrer', () => {
    const result = parseCampaign('?utm_source=instagram&utm_medium=paid_social&utm_campaign=hive_opening&email=test%40example.com&gclid=xyz');
    expect(result).toEqual({
      utmSource: 'instagram', utmMedium: 'paid_social', utmCampaign: 'hive_opening', utmContent: '', utmTerm: '',
    });
    expect(JSON.stringify(result)).not.toContain('example.com');
    expect(JSON.stringify(result)).not.toContain('gclid');
  });
  it('discards invalid control characters and oversized fields', () => {
    expect(parseCampaign('?utm_source=' + 'x'.repeat(81))).toBeNull();
    expect(parseCampaign('?utm_source=ok%0Ainvalid')).toBeNull();
  });
  it('preserves a campaign in per-tab storage across navigation', () => {
    const store = new Map<string, string>();
    const storage = { getItem: (k: string) => store.get(k) || null, setItem: (k: string, v: string) => { store.set(k, v); } };
    expect(captureCampaign('?utm_source=meta&utm_campaign=community', storage)?.utmSource).toBe('meta');
    expect(captureCampaign('?mode=signup', storage)).toBeNull();
    expect(savedCampaign(storage)?.utmCampaign).toBe('community');
    expect(savedCampaign({ getItem: () => '{broken' })).toBeNull();
  });
});
