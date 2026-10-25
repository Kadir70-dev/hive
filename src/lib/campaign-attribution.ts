/** First-party, per-tab campaign attribution. No cookies, IP, names, emails or referrers. */
export type CampaignAttribution = {
  utmSource: string; utmMedium: string; utmCampaign: string; utmContent: string; utmTerm: string;
};
const STORE = 'hive.campaign.v1';
const keys = { utm_source: 'utmSource', utm_medium: 'utmMedium', utm_campaign: 'utmCampaign', utm_content: 'utmContent', utm_term: 'utmTerm' } as const;
function clean(value: string | null): string {
  const v = (value || '').trim();
  return v.length <= 80 && !/[\u0000-\u001f\u007f]/.test(v) ? v : '';
}
export function parseCampaign(search: string): CampaignAttribution | null {
  const params = new URLSearchParams(search.startsWith('?') ? search : '?' + search);
  const result = {} as CampaignAttribution;
  for (const [input, output] of Object.entries(keys) as [keyof typeof keys, keyof CampaignAttribution][]) {
    result[output] = clean(params.get(input));
  }
  return Object.values(result).some(Boolean) ? result : null;
}
export function captureCampaign(search: string, storage: Pick<Storage, 'setItem'>): CampaignAttribution | null {
  const campaign = parseCampaign(search);
  if (campaign) storage.setItem(STORE, JSON.stringify(campaign));
  return campaign;
}
export function savedCampaign(storage: Pick<Storage, 'getItem'>): CampaignAttribution | null {
  try {
    const parsed: unknown = JSON.parse(storage.getItem(STORE) || 'null');
    if (!parsed || typeof parsed !== 'object') return null;
    const val = parsed as Record<string, unknown>;
    const result = {} as CampaignAttribution;
    for (const field of Object.values(keys)) result[field] = clean(typeof val[field] === 'string' ? val[field] : null);
    return Object.values(result).some(Boolean) ? result : null;
  } catch { return null; }
}
