import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { metadata } from '@/app/layout';

const OLD_LOVABLE_FAVICON_BYTES = 20373;

describe('Hive brand icons', () => {
  it('serves the sage-circle H mark, not the old Lovable favicon', () => {
    const svg = readFileSync('public/icon-mark.svg', 'utf8');
    expect(svg).toContain('<circle cx="256" cy="256" r="256" fill="#3F6E5E"/>');
    expect(svg).toContain('fill="#FFFCF7"');
    expect(readFileSync('public/favicon.ico').length).not.toBe(OLD_LOVABLE_FAVICON_BYTES);
  });

  it('versions every icon URL so browsers refetch the tab icon', () => {
    const icons = JSON.stringify(metadata.icons);
    expect(icons).toContain('/favicon.ico?v=2');
    expect(icons).toContain('/icon-mark.svg?v=2');
    expect(icons).toContain('/apple-touch-icon.png?v=2');
    const manifest = JSON.parse(readFileSync('public/manifest.webmanifest', 'utf8')) as { icons: { src: string }[] };
    for (const icon of manifest.icons) expect(icon.src).toMatch(/\?v=2$/);
    expect(readFileSync('public/hive.html', 'utf8')).toContain('<link rel="icon" href="/favicon.ico?v=2"');
  });
});
