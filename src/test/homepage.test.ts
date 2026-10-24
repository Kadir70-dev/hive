import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const html = readFileSync(path.join(process.cwd(), 'public/hive.html'), 'utf8');

describe('public homepage', () => {
  it('does not use browser-stored third-party authentication tokens', () => {
    expect(html).not.toMatch(/sb-.*auth-token/i);
    expect(html).not.toMatch(/(?:localStorage|sessionStorage)\.(?:getItem|setItem)\(\s*["'](?:auth[_-]?token|access[_-]?token|session[_-]?token)/i);
    expect(html.toLowerCase()).not.toContain('lovable');
  });

  it('detects members through the Go-backed session endpoint', () => {
    expect(html).toContain('fetch("/api/backend/me",{credentials:"same-origin"');
  });

  it('keeps the AI chat and recommendation widgets disabled', () => {
    expect(html).toMatch(/<section id="ask"[^>]*\bhidden\b[^>]*display:none/);
    expect(html).toContain('const ASK_ENABLED=false;');
    expect(html).toContain('if(ASK_ENABLED)ask(interestIn.value)');
  });

  it('sends Join to the sign-up form', () => {
    expect(html).toContain('href="/auth?mode=signup"');
  });
});
