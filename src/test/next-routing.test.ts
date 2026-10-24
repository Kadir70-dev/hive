import { describe,expect,it } from 'vitest';
import { existsSync } from 'node:fs';
import path from 'node:path';
// Account deletion is DELETE /api/backend/me (proxied to the Go API's DELETE /v1/me);
// there is intentionally no separate /api/members/delete route.
const routes=['src/app/page.tsx','src/app/layout.tsx','src/app/auth/page.tsx','src/app/privacy/page.tsx',
'src/app/guidelines/page.tsx','src/app/terms/page.tsx','src/app/reset-password/page.tsx','src/app/verify-email/page.tsx','src/app/(member)/members/page.tsx',
'src/app/(member)/gatherings/page.tsx','src/app/(member)/profile/page.tsx','src/app/(member)/admin/page.tsx',
'src/app/(member)/announcements/page.tsx','src/app/(member)/partners/page.tsx','src/app/(member)/safety/page.tsx',
'src/app/api/backend/[...path]/route.ts'];
describe('Next App Router migration',()=>{
  it('preserves all public, private, and API routes',()=>{
    for(const pathname of routes)expect(existsSync(path.join(process.cwd(),pathname)),pathname).toBe(true);
  });
  it('does not resurrect removed routes (delete, Lovable preview, AI stubs)',()=>{
    for(const gone of ['src/app/api/members/delete/route.ts','src/app/lovable','src/app/api/public/chat','src/app/api/public/recommendations','src/app/api/public/signup','src/app/api/members/signup','src/lib/ai','src/lib/email-templates'])
      expect(existsSync(path.join(process.cwd(),gone)),gone).toBe(false);
  });
});
