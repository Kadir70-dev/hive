import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { DICT } from '@/lib/hive-i18n';

const profile = readFileSync('src/components/screens/ProfileScreen.tsx', 'utf8');

describe('Hive membership branding and profile layout', () => {
  it('uses organisation branding in pending-member messaging', () => {
    expect(DICT.en.pendingText).toContain('The Hive Society team');
    expect(DICT.en.pendingText).not.toMatch(/Meera reads/i);
    expect(DICT.ar.pendingText).not.toMatch(/Meera/i);
  });

  it('shows membership status prominently before editable details', () => {
    expect(profile).toContain('hv-profile-summary');
    expect(profile).toContain('hv-profile-grid');
    expect(profile.indexOf('hv-profile-summary')).toBeLessThan(profile.indexOf('hv-profile-grid'));
    expect(profile).toContain('t.profileDetails');
    expect(profile).toContain('MyGatherings status={me.status}');
  });

  it('does not show an unavailable photo-uploader card', () => {
    expect(profile).toContain('me.photos_enabled && <ProfilePhoto me={me} />');
  });

  it('preserves profile changes and account deletion endpoints', () => {
    expect(profile).toContain("await send('/me', { firstName: name");
    expect(profile).toContain("await send('/me', { password:");
    expect(profile).toContain("await qc.invalidateQueries({ queryKey: ['me'] })");
  });
});
