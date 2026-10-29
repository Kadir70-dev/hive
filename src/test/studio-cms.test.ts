import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { diffDoc, emptySEO, extractCopy, mergeCopy, renderHome, safeImageURL } from '@/lib/cms-home';
import { STUDIO_PAGES, studioKeys } from '@/lib/studio-sections';
import { inlineScriptHashes, staticHomepageCSP } from '@/lib/security-headers';

const template = readFileSync(path.join(process.cwd(), 'public/hive.html'), 'utf8');
const base = extractCopy(template);
const copyOf = (html: string) => extractCopy(html);

describe('homepage copy block', () => {
  it('holds the English and Arabic website copy the page paints from', () => {
    expect(base.en.heroTitle).toBe('The art of <em>getting together</em>');
    expect(base.en.heroIntro).toBe('An Abu Dhabi community. Discover free, curated gatherings — with dedicated women-only experiences.');
    expect(String(base.ar.heroIntro)).toContain('أبوظبي');
    expect(Array.isArray(base.en.faq)).toBe(true);
    expect(template).toContain('const L=JSON.parse(document.getElementById("hive-copy").textContent);');
    expect(template).toContain('data-img="visionImg1"');
  });

  it('every Studio field is a real homepage copy key', () => {
    const missing = studioKeys(STUDIO_PAGES[0]).filter((k) => !(k in base.en));
    expect(missing).toEqual([]);
  });

  it('covers every text the homepage paints with data-t', () => {
    const painted = new Set([...template.matchAll(/data-t="([A-Za-z0-9]+)"/g)].map((m) => m[1]));
    const editable = new Set(studioKeys(STUDIO_PAGES[0]));
    expect([...painted].filter((k) => !editable.has(k))).toEqual([]);
  });
});

describe('publishing copy into the homepage', () => {
  it('applies overrides and leaves everything else as shipped', () => {
    const html = renderHome(template, { en: { heroLine: 'QA-TEST line', faq: [['QA-TEST q', 'QA-TEST a']] }, ar: { heroLine: 'QA-TEST سطر' } });
    const copy = copyOf(html);
    expect(copy.en.heroLine).toBe('QA-TEST line');
    expect(copy.ar.heroLine).toBe('QA-TEST سطر');
    expect(copy.en.faq).toEqual([['QA-TEST q', 'QA-TEST a']]);
    expect(copy.en.heroIntro).toBe(base.en.heroIntro);
  });

  it('cannot inject markup or scripts', () => {
    const html = renderHome(template, { en: {
      heroTitle: 'QA <em>ok</em> <img src=x onerror=alert(1)>',
      heroLine: '</script><script>alert(1)</script>',
      visionImg1: 'javascript:alert(1)', visionImg2: 'https://x.test/a.jpg") ;x', langBtn: '<b>x</b>', notAKey: 'x',
    } });
    const copy = copyOf(html);
    expect(copy.en.heroTitle).toBe('QA <em>ok</em> &lt;img src=x onerror=alert(1)&gt;');
    expect(copy.en.heroLine).toBe('</script><script>alert(1)</script>'); // painted as textContent
    expect(html).not.toContain('</script><script>alert(1)');
    expect(copy.en.visionImg1).toBe('');
    expect(copy.en.visionImg2).toBe('');
    expect(copy.en.langBtn).toBe(base.en.langBtn);
    expect('notAKey' in copy.en).toBe(false);
    // The copy block is JSON data, so the page CSP hashes are unchanged by publishing.
    expect(inlineScriptHashes(html)).toEqual(inlineScriptHashes(template));
    expect(staticHomepageCSP(html, { httpsImages: true })).toContain("img-src 'self' data: https:");
  });

  it('accepts only site paths and https image URLs', () => {
    for (const ok of ['', '/api/backend/cms/media/abc', 'https://images.example.test/a.webp']) expect(safeImageURL(ok)).toBe(true);
    for (const bad of ['//evil.test/a', 'http://x.test/a', 'javascript:alert(1)', 'https://x.test/a b', 'https://x.test/")']) expect(safeImageURL(bad)).toBe(false);
  });

  it('saves only what the editor changed', () => {
    const working = mergeCopy(base, {});
    working.en.heroLine = 'QA-TEST changed';
    const doc = diffDoc(base, working, emptySEO());
    expect(doc.en).toEqual({ heroLine: 'QA-TEST changed' });
    expect(doc.ar).toEqual({});
  });
});
