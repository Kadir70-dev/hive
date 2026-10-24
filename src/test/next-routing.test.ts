import { describe,expect,it } from 'vitest';
import { existsSync } from 'node:fs';
import path from 'node:path';
// Account deletion is DELETE /api/backend/me (proxied to the Go API's DELETE /v1/me);
// there is intentionally no separate /api/members/delete route.
const routes=['src/app/page.tsx','src/app/layout.tsx','src/app/auth/page.tsx','src/app/privacy/page.tsx',
'src/app/guidelines/page.tsx','src/app/terms/page.tsx','src/app/reset-password/page.tsx','src/app/(member)/members/page.tsx',
'src/app/(member)/gatherings/page.tsx','src/app/(member)/profile/page.tsx','src/app/(member)/admin/page.tsx',
'src/app/(member)/announcements/page.tsx','src/app/(member)/partners/page.tsx','src/app/(member)/safety/page.tsx',
'src/app/api/backend/[...path]/route.ts','src/app/api/public/signup/route.ts','src/app/api/members/signup/route.ts'];
describe('Next App Router migration',()=>{
  it('preserves all public, private, and API routes',()=>{
    for(const pathname of routes)expect(existsSync(path.join(process.cwd(),pathname)),pathname).toBe(true);
  });
  it('does not resurrect the removed delete route',()=>{
    expect(existsSync(path.join(process.cwd(),'src/app/api/members/delete/route.ts'))).toBe(false);
  });
});
