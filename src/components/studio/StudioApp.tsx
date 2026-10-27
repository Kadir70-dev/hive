'use client';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api, ApiError, send } from '@/lib/hive-api';
import { isImageKey, type Lang, type Pair } from '@/lib/cms-home';
import { STUDIO_PAGES } from '@/lib/studio-sections';
import SectionCanvas from './SectionCanvas';
import SeoCanvas from './SeoCanvas';
import MediaManager from './MediaManager';
import { StudioBlocked, StudioLogin, StudioMfa } from './StudioLogin';
import { useStudioPage } from './useStudioPage';
import './studio.css';

type Me = { isAdmin: boolean; email_verified: boolean; mfa_enabled: boolean; mfa_verified: boolean };
type Auth = 'loading' | 'login' | 'mfa' | 'not_admin' | 'unverified' | 'no_mfa' | 'ready';
type Picking = { key: string; alt?: string } | null;

const PAGE = STUDIO_PAGES[0];
const MEDIA = 'media';
const ORDER = [...PAGE.sections.map((s) => s.id), MEDIA];
const errorText = (e: unknown, fallback: string) => (e instanceof Error ? e.message : fallback);

/** The section named in ?section=…, so a reload keeps the editor's place. */
function initialSection(): string {
  if (typeof window === 'undefined') return ORDER[0];
  const s = new URLSearchParams(window.location.search).get('section');
  return s && ORDER.includes(s) ? s : ORDER[0];
}

function authFrom(me: Me): Auth {
  if (!me.isAdmin) return 'not_admin';
  if (!me.email_verified) return 'unverified';
  if (!me.mfa_enabled) return 'no_mfa';
  return me.mfa_verified ? 'ready' : 'mfa';
}

export default function StudioApp() {
  const session = useQuery({ queryKey: ['me'], queryFn: () => api<Me>('/me'), retry: false });
  const auth: Auth = session.isPending ? 'loading' : session.data ? authFrom(session.data) : 'login';
  const { refetch } = session;
  const checkSession = useCallback(() => { void refetch(); }, [refetch]);
  const [current, setCurrent] = useState(initialSection), [lang, setLang] = useState<Lang>('en');
  const [busy, setBusy] = useState(false), [message, setMessage] = useState(''), [error, setError] = useState('');
  const [history, setHistory] = useState(false), [picking, setPicking] = useState<Picking>(null);
  const host = typeof window === 'undefined' ? '' : window.location.host;
  const page = useStudioPage(PAGE.slug, '/hive.html');
  const { load, dirty } = page;
  const section = PAGE.sections.find((s) => s.id === current);
  const index = ORDER.indexOf(current);

  useEffect(() => {
    if (auth !== 'ready' || page.ready) return;
    load().catch((e) => {
      if (e instanceof ApiError && (e.status === 401 || e.code === 'mfa_required')) checkSession();
      setError(errorText(e, 'Could not load the page'));
    });
  }, [auth, page.ready, load, checkSession]);
  // Leaving the Studio (reload, close, back) with unsaved edits asks first.
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ''; };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  function go(id: string) {
    setCurrent(id); setMessage(''); setError(''); setHistory(false);
    try { const u = new URL(location.href); u.searchParams.set('section', id); window.history.replaceState(null, '', u); } catch { /* address bar update is optional */ }
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }
  async function logout() {
    if (dirty && !window.confirm('You have unsaved changes. Log out without saving?')) return;
    try { await send('/logout', {}); } catch { /* the session is cleared below either way */ }
    window.location.reload();
  }
  async function run(work: () => Promise<unknown>, done: string, fail: string) {
    setBusy(true); setError(''); setMessage('');
    try { await work(); setMessage(done); }
    catch (e) {
      if (e instanceof ApiError && (e.status === 401 || e.code === 'mfa_required')) checkSession();
      setError(errorText(e, fail));
    } finally { setBusy(false); }
  }
  const save = () => run(page.saveDraft, 'Draft saved. The live website is unchanged until you publish.', 'Save failed');
  const publish = () => run(page.publish, 'Published. The live homepage now shows these changes.', 'Publish failed');
  function restore(v: number) {
    if (dirty && !window.confirm('Restoring replaces your unsaved changes. Continue?')) return;
    if (!window.confirm(`Restore version ${v}? It becomes the live homepage straight away.`)) return;
    void run(() => page.restore(v), `Version ${v} restored and live.`, 'Restore failed');
  }

  const text = useCallback((key: string) => { const v = page.value(lang, key); return typeof v === 'string' ? v : ''; }, [page, lang]);
  const pairs = useCallback((key: string) => { const v = page.value(lang, key); return Array.isArray(v) ? v : []; }, [page, lang]);
  // Images are shared by both languages, so they are always stored with the English copy.
  const onText = useCallback((key: string, v: string) => page.set(isImageKey(key) ? 'en' : lang, key, v), [page, lang]);
  const onPairs = useCallback((key: string, v: Pair[]) => page.set(lang, key, v), [page, lang]);
  function applyImage(url: string, alt: string) {
    if (!picking) return;
    if (picking.key === 'seo') page.setSeo('og_image', url);
    else { page.set('en', picking.key, url); if (picking.alt && alt) page.set(lang, picking.alt, alt); }
    setMessage('Image changed. Save or publish to keep it.'); setPicking(null);
  }
  const status = useMemo(() => dirty ? <b className="st-dirty">● Unsaved changes</b> : <span>All changes saved{page.draftIsLive ? '' : ' · draft not live yet'}</span>, [dirty, page.draftIsLive]);

  if (auth === 'loading') return <main className="studio st-center"><p className="st-muted">Loading…</p></main>;
  if (auth === 'login') return <StudioLogin onDone={checkSession} />;
  if (auth === 'mfa') return <StudioMfa onDone={checkSession} onLogout={() => void logout()} />;
  if (auth !== 'ready') return <StudioBlocked reason={auth} onLogout={() => void logout()} />;

  return <main className="studio">
    <div className="st-wrap">
      <header className="st-top">
        <strong>The Hive Society <span>· Studio</span></strong>
        <nav aria-label="Studio">
          <a href="/" target="_blank" rel="noopener">View website ↗</a>
          <a href="/admin" target="_blank" rel="noopener">Dashboard ↗</a>
          <button type="button" className="st-logout" onClick={() => void logout()}>Logout</button>
        </nav>
      </header>
      <nav className="st-nav" aria-label="Website pages and sections">
        <div className="st-page-row">
          <button type="button" className={`st-page${section ? ' active' : ''}`} aria-current={section ? 'page' : undefined} onClick={() => go(ORDER[0])}>{PAGE.name}</button>
          <button type="button" className={`st-page${current === MEDIA ? ' active' : ''}`} aria-current={current === MEDIA ? 'page' : undefined} onClick={() => go(MEDIA)}>Image Uploads &amp; Url</button>
        </div>
        {section && <div className="st-pill-row">
          {PAGE.sections.map((s) => <button key={s.id} type="button" className={`st-pill${s.id === current ? ' active' : ''}`} aria-current={s.id === current ? 'true' : undefined} onClick={() => go(s.id)}>{s.name}</button>)}
          {!section.seo && <span className="st-lang" role="group" aria-label="Language">
            {(['en', 'ar'] as const).map((l) => <button key={l} type="button" aria-pressed={lang === l} onClick={() => setLang(l)}>{l === 'en' ? 'English' : 'العربية'}</button>)}
          </span>}
        </div>}
      </nav>
      {error && <p className="st-error" role="alert">{error}</p>}
      {message && <p className="st-note" role="status">{message}</p>}
      <div key={current + lang} className="st-stage">
        {!section ? <MediaManager /> : !page.ready ? <p className="st-muted st-loading">Loading the live homepage…</p> : section.seo
          ? <SeoCanvas seo={page.seo} host={host} onSeo={page.setSeo} onPickImage={() => setPicking({ key: 'seo' })} />
          : <SectionCanvas section={section} lang={lang} text={text} pairs={pairs} onText={onText} onPairs={onPairs} isHtml={page.isHtml}
            onPickImage={(key, alt) => setPicking({ key, alt })} />}
      </div>
      {section && page.ready && <div className="st-savebar">
        <div className="st-status">{status}<span>Draft #{page.revs.draft} · Live #{page.revs.live}</span></div>
        <div className="st-save-actions">
          <button type="button" className="st-ghost" aria-expanded={history} onClick={() => setHistory((h) => !h)}>History</button>
          <a className="st-ghost" href={PAGE.path} target="_blank" rel="noopener">View page ↗</a>
          <button type="button" className="st-ghost" disabled={busy || !dirty} onClick={() => void save()}>Save draft</button>
          <button type="button" className="st-publish" disabled={busy} onClick={() => void publish()}>{busy ? 'Saving…' : 'Publish live'}</button>
        </div>
        {history && <ul className="st-history" aria-label="Published versions">
          {page.versions.length ? page.versions.map((v) => <li key={v.version}>
            <span>Version {v.version} · {new Date(v.created_at).toLocaleString()}{v.published_by ? ` · ${v.published_by}` : ''}{v.version === page.revs.live ? ' · live' : ''}</span>
            <button type="button" disabled={busy || v.version === page.revs.live} onClick={() => restore(v.version)}>Restore</button>
          </li>) : <li>No published versions yet.</li>}
        </ul>}
      </div>}
      <div className="st-pager">
        <button type="button" disabled={index <= 0} onClick={() => go(ORDER[index - 1])}>Previous</button>
        <button type="button" className="next" disabled={index >= ORDER.length - 1} onClick={() => go(ORDER[index + 1])}>Next</button>
      </div>
    </div>
    {picking && <div className="st-modal" role="dialog" aria-modal="true" aria-label="Choose an image" onClick={(e) => { if (e.target === e.currentTarget) setPicking(null); }}>
      <MediaManager onPick={({ url, alt }) => applyImage(url, alt)} onClose={() => setPicking(null)} />
    </div>}
  </main>;
}
