import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const source = (p: string) => readFileSync(join(process.cwd(), p), 'utf8');

describe('Batch 5 private photo wiring', () => {
  it('posts multipart FormData without overriding its boundary', () => {
    const photo = source('src/components/screens/ProfilePhoto.tsx');
    expect(photo).toContain("payload.append('photo', selected, selected.name)");
    expect(photo).toContain("fetch('/api/backend/me/photo'");
    expect(photo).not.toMatch(/headers:\\s*\\{\\s*['\"]Content-Type['\"]/);
    expect(photo).toContain("await qc.invalidateQueries({ queryKey: ['me'] })");
  });

  it('keeps private photos behind API routes rather than public S3 URLs', () => {
    const profile = source('src/components/screens/ProfilePhoto.tsx');
    const admin = source('src/components/admin/AdminMembers.tsx');
    expect(profile).toContain('/api/backend/me/photo');
    expect(admin).toContain('/api/backend/admin/members/');
    expect(profile).not.toContain('R2_SECRET_ACCESS_KEY');
  });

  it('routes the safety conflict UI through admin endpoints only', () => {
    const conflict = source('src/components/admin/AdminSafetyConflicts.tsx');
    expect(conflict).toContain("'/admin/safety-conflicts'");
    expect(conflict).toContain("'/admin/safety-conflicts/resolve'");
    expect(source('src/components/screens/AdminScreen.tsx')).toContain('<MfaGate me={me}>');
  });

  it('limits large multipart uploads and blocks public metrics in Caddy', () => {
    const caddy = source('Caddyfile');
    expect(caddy).toContain('path /internal /internal/*');
    expect(caddy).toContain('path /v1/me/photo');
    expect(caddy).toContain('max_size 6MB');
    expect(caddy).toContain('max_size 1MB');
  });
});
