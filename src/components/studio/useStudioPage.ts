'use client';
import { useCallback, useMemo, useState } from 'react';
import { api, send } from '@/lib/hive-api';
import { diffDoc, emptySEO, extractCopy, mergeCopy, type CmsDoc, type CmsSEO, type Copy, type CopyValue, type Lang } from '@/lib/cms-home';

type PageReply = { draft: CmsDoc; published: CmsDoc; draft_revision: number; published_revision: number; published_at: string | null };
export type Version = { version: number; created_at: string; published_by: string };

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
const seoOf = (d: Partial<CmsDoc>): CmsSEO => ({ ...emptySEO(), ...(d.seo ?? {}) });

/**
 * Loads one page's built-in copy, saved draft and history together, so the
 * canvas only appears once the real website text is in place.
 */
export function useStudioPage(slug: string, templatePath: string) {
  const [base, setBase] = useState<Copy | null>(null);
  const [working, setWorking] = useState<Copy | null>(null);
  const [seo, setSeoState] = useState<CmsSEO>(emptySEO());
  const [saved, setSaved] = useState<CmsDoc | null>(null), [live, setLive] = useState<CmsDoc | null>(null);
  const [revs, setRevs] = useState({ draft: 0, live: 0 }), [versions, setVersions] = useState<Version[]>([]);

  const load = useCallback(async () => {
    const [html, page, history] = await Promise.all([
      fetch(templatePath, { cache: 'no-store' }).then((r) => { if (!r.ok) throw new Error('Could not read the website copy'); return r.text(); }),
      api<PageReply>(`/admin/cms/pages/${slug}`),
      api<{ data: Version[] }>(`/admin/cms/pages/${slug}/versions`),
    ]);
    const b = extractCopy(html), w = mergeCopy(b, page.draft), s = seoOf(page.draft);
    setBase(b); setWorking(w); setSeoState(s);
    setSaved(diffDoc(b, w, s)); setLive(diffDoc(b, mergeCopy(b, page.published), seoOf(page.published)));
    setRevs({ draft: page.draft_revision, live: page.published_revision }); setVersions(history.data);
  }, [slug, templatePath]);

  const current = useMemo(() => (base && working ? diffDoc(base, working, seo) : null), [base, working, seo]);
  const dirty = !!current && !!saved && !same(current, saved);
  const draftIsLive = !!saved && !!live && same(saved, live);

  const set = useCallback((lang: Lang, key: string, v: CopyValue) => setWorking((w) => (w ? { ...w, [lang]: { ...w[lang], [key]: v } } : w)), []);
  const setSeo = useCallback((key: keyof CmsSEO, v: string) => setSeoState((s) => ({ ...s, [key]: v })), []);
  const value = useCallback((lang: Lang, key: string): CopyValue | undefined => working?.[lang][key] ?? (lang === 'ar' ? working?.en[key] : undefined), [working]);
  const isHtml = useCallback((key: string) => typeof base?.en[key] === 'string' && (base.en[key] as string).includes('<em>'), [base]);

  const saveDraft = useCallback(async () => {
    if (!current) return revs.draft;
    const r = await send<{ draft_revision: number }>(`/admin/cms/pages/${slug}/draft`, { document: current, expected_revision: revs.draft }, 'PUT');
    setSaved(current); setRevs((x) => ({ ...x, draft: r.draft_revision }));
    return r.draft_revision;
  }, [current, revs.draft, slug]);

  const publish = useCallback(async () => {
    const rev = dirty || revs.draft === 0 ? await saveDraft() : revs.draft;
    await send(`/admin/cms/pages/${slug}/publish`, { expected_revision: rev });
    await load();
  }, [dirty, revs.draft, saveDraft, slug, load]);

  const restore = useCallback(async (version: number) => {
    await send(`/admin/cms/pages/${slug}/restore`, { version });
    await load();
  }, [slug, load]);

  return { ready: !!working, load, value, set, seo, setSeo, isHtml, dirty, draftIsLive, revs, versions, saveDraft, publish, restore };
}
