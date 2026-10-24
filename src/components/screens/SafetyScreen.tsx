'use client';
import { useState, type FormEvent } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, send } from '@/lib/hive-api';
import { HiveShell, useT } from '@/components/hive/HiveShell';
import { Empty, ErrorNote, Loading, Notice } from '@/components/hive/ui';

type MyGathering = { id: string; title: string; starts_on: string };
type MyReport = { id: string; category: string; gathering_title: string; status: string; created_at: string };

/**
 * Admin-mediated safety reporting: members describe who was involved instead
 * of choosing another member, so no member identities are ever shown here.
 */
function SafetyBody() {
  const { t } = useT();
  const qc = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<{ message: string; tone: 'ok' | 'err' }>({ message: '', tone: 'ok' });
  const gatherings = useQuery({ queryKey: ['my-gatherings'], queryFn: () => api<MyGathering[]>('/me/gatherings') });
  const reports = useQuery({ queryKey: ['my-reports'], queryFn: () => api<MyReport[]>('/reports/mine') });
  const categories: [string, string][] = [['harassment', t.catHarassment], ['safety', t.catSafety], ['inappropriate', t.catInappropriate], ['other', t.catOther]];
  const statusLabel: Record<string, string> = { open: t.rsOpen, in_review: t.rsInReview, resolved: t.rsResolved, dismissed: t.rsDismissed };
  const categoryLabel = Object.fromEntries(categories);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    setBusy(true);
    try {
      await send('/reports', {
        category: String(f.get('category') || 'other'), gatheringID: String(f.get('gathering') || ''),
        subjectDescription: String(f.get('who') || ''), reason: String(f.get('what') || ''), blockRequested: f.get('block') === 'on',
      });
      form.reset();
      setNotice({ message: t.sSent, tone: 'ok' });
      void qc.invalidateQueries({ queryKey: ['my-reports'] });
    } catch (err) { setNotice({ message: err instanceof Error ? err.message : t.loadError, tone: 'err' }); } finally { setBusy(false); }
  }

  return (
    <>
      <div className="hv-hero"><h1>{t.sTitle}</h1><p>{t.sLead}</p></div>
      <div className="hv-card">
        <form className="hv-stack" onSubmit={submit}>
          <label>{t.sCategory}
            <select name="category" defaultValue="harassment">{categories.map(([v, l]) => <option key={v} value={v}>{l}</option>)}</select>
          </label>
          <label>{t.sGathering}
            <select name="gathering" defaultValue="">
              <option value="">{t.sNoGathering}</option>
              {(gatherings.data ?? []).map((g) => <option key={g.id} value={g.id}>{g.title} · {g.starts_on}</option>)}
            </select>
          </label>
          <label>{t.sWho}
            <input name="who" maxLength={500} aria-describedby="who-hint" />
            <span id="who-hint" className="hv-field-hint">{t.sWhoHint}</span>
          </label>
          <label>{t.sWhat}<textarea name="what" required minLength={10} maxLength={2000} rows={5} /></label>
          <label className="hv-check"><input type="checkbox" name="block" /> <span>{t.sBlock}</span></label>
          <Notice message={notice.message} tone={notice.tone} />
          <button className="hv-btn" disabled={busy}>{busy ? t.loading : t.sSend}</button>
        </form>
      </div>
      <div className="hv-card">
        <h2>{t.sMine}</h2>
        {reports.isLoading && <Loading label={t.loading} />}
        {reports.error && <ErrorNote message={t.loadError} onRetry={() => void reports.refetch()} retryLabel={t.retry} />}
        {reports.data?.length === 0 && <Empty>{t.sMineNone}</Empty>}
        {reports.data?.map((r) => (
          <div className="hv-item" key={r.id}>
            <div className="hv-item-head">
              <span>{categoryLabel[r.category] ?? r.category}{r.gathering_title ? ` · ${r.gathering_title}` : ''}</span>
              <span className="hv-pill">{statusLabel[r.status] ?? r.status}</span>
            </div>
            <span className="hv-muted">{r.created_at.slice(0, 10)}</span>
          </div>
        ))}
      </div>
    </>
  );
}

export default function SafetyScreen() {
  return <HiveShell member><SafetyBody /></HiveShell>;
}
