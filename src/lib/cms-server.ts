import 'server-only';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { forwardToApi } from '@/lib/backend-proxy';
import { emptyDoc, type CmsDoc } from '@/lib/cms-home';

let template: Promise<string> | null = null;
/** The homepage template shipped in public/ (read once per server process). */
export function homeTemplate(): Promise<string> {
  template ??= readFile(path.join(process.cwd(), 'public/hive.html'), 'utf8').catch((e) => { template = null; throw e; });
  return template;
}

/**
 * Published Studio overrides for a page. Any failure falls back to the
 * built-in copy, so the website never breaks because the API is down.
 */
export async function publishedDoc(req: Request, slug = 'home'): Promise<CmsDoc> {
  try {
    const res = await forwardToApi(new Request(req.url, { headers: req.headers }), `/v1/cms/pages/${slug}`);
    if (!res.ok) return emptyDoc();
    const body = (await res.json()) as { document?: Partial<CmsDoc> };
    const doc = body.document ?? {};
    return { en: doc.en ?? {}, ar: doc.ar ?? {}, seo: { ...emptyDoc().seo, ...(doc.seo ?? {}) } };
  } catch {
    return emptyDoc();
  }
}
