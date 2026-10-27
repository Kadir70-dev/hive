/**
 * Website Studio content for the homepage (public/hive.html).
 *
 * The homepage keeps all of its English and Arabic copy in a JSON block
 * (#hive-copy). A CMS document only overrides some of those values; the
 * server merges the published overrides into the block before the page is
 * sent, so visitors and search engines see the live copy with no flash.
 */
export type Lang = 'en' | 'ar';
export type Pair = [string, string];
export type CopyValue = string | Pair[];
export type Copy = Record<Lang, Record<string, CopyValue>>;
export type CmsSEO = { title: string; description: string; og_image: string; canonical: string; robots: '' | 'index' | 'noindex' };
export type CmsDoc = { en: Record<string, CopyValue>; ar: Record<string, CopyValue>; seo: CmsSEO };

export const LANGS: Lang[] = ['en', 'ar'];
export const emptySEO = (): CmsSEO => ({ title: '', description: '', og_image: '', canonical: '', robots: '' });
export const emptyDoc = (): CmsDoc => ({ en: {}, ar: {}, seo: emptySEO() });

const COPY_BLOCK = /(<script type="application\/json" id="hive-copy">)([\s\S]*?)(<\/script>)/;

/** Reads the built-in copy from the homepage template. */
export function extractCopy(html: string): Copy {
  const m = html.match(COPY_BLOCK);
  if (!m) throw new Error('homepage copy block missing');
  const parsed = JSON.parse(m[2]) as Copy;
  return { en: parsed.en ?? {}, ar: parsed.ar ?? {} };
}

/** Image slots hold a site path or an https URL that is safe inside CSS url("…"). */
export const isImageKey = (key: string) => /Img[0-9]?$/.test(key);
export function safeImageURL(v: string): boolean {
  if (v === '') return true;
  if (v.length > 2048 || /["'()\\<>\s]/.test(v)) return false;
  if (v.startsWith('/')) return !v.startsWith('//');
  try { const u = new URL(v); return u.protocol === 'https:' && !!u.host && !u.username && !u.password; } catch { return false; }
}

const escapeHTML = (s: string) => s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
/** Plain text with optional <em>…</em> emphasis; any other markup is escaped. */
export const emphasisOnly = (s: string) => escapeHTML(s).replace(/&lt;(\/?)em&gt;/g, '<$1em>');

function cleanValue(key: string, base: CopyValue | undefined, v: unknown): CopyValue | undefined {
  if (typeof v === 'string') {
    if (isImageKey(key)) return safeImageURL(v) ? v : undefined;
    // Headings the homepage writes with innerHTML keep <em> only; every other key
    // is written as textContent. The API enforces the same rule on save.
    return typeof base === 'string' && /<em>/.test(base) ? emphasisOnly(v) : v;
  }
  if (Array.isArray(v) && v.every((p) => Array.isArray(p) && p.length === 2 && p.every((x) => typeof x === 'string'))) return v as Pair[];
  return undefined;
}

/** Built-in copy with a document's overrides applied (unknown keys are ignored). */
export function mergeCopy(base: Copy, doc: Partial<CmsDoc> | null | undefined): Copy {
  const out: Copy = { en: { ...base.en }, ar: { ...base.ar } };
  for (const lang of LANGS) {
    for (const [key, v] of Object.entries(doc?.[lang] ?? {})) {
      if (!(key in base.en) || key === 'langBtn') continue;
      const clean = cleanValue(key, base.en[key], v);
      if (clean !== undefined) out[lang][key] = clean;
    }
  }
  return out;
}

/** The homepage HTML with the merged copy written into its JSON block. */
export function renderHome(template: string, doc: Partial<CmsDoc> | null | undefined): string {
  const json = JSON.stringify(mergeCopy(extractCopy(template), doc)).replace(/</g, '\\u003c').replace(/\u2028/g, '\\u2028').replace(/\u2029/g, '\\u2029');
  return template.replace(COPY_BLOCK, (_, open: string, _old: string, close: string) => open + json + close);
}

/** Only the values that differ from the built-in copy, so documents stay small. */
export function diffDoc(base: Copy, working: Copy, seo: CmsSEO): CmsDoc {
  const doc = emptyDoc();
  for (const lang of LANGS) {
    for (const [key, v] of Object.entries(working[lang])) {
      const original = base[lang][key] ?? (lang === 'ar' ? undefined : base.en[key]);
      if (JSON.stringify(v) !== JSON.stringify(original)) doc[lang][key] = v;
    }
  }
  doc.seo = { ...seo };
  return doc;
}
