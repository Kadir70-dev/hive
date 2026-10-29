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

  it('removes the member chat and AI conversation interface entirely', () => {
    expect(html).not.toContain('id="ask"');
    expect(html).not.toContain('id="chat"');
    expect(html).not.toContain('/api/public/chat');
    expect(html).not.toContain('drawChat()');
  });

  it('welcomes all nationalities and genders and distinguishes gathering access', () => {
    expect(html).toContain('An Abu Dhabi community. Discover free, curated gatherings — with dedicated women-only experiences.');
    expect(html).not.toContain('An Abu Dhabi community for men and women of all nationalities.');
    expect(html).toContain('Mixed-gender gatherings');
    expect(html).toContain('Women-only gatherings');
    expect(html).not.toContain('For the women of Abu Dhabi');
    expect(html).toContain('gatheringFreeNote');
  });

  it('sends Join to the sign-up form', () => {
    expect(html).toContain('href="/auth?mode=signup"');
  });
});
