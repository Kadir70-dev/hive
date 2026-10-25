'use client';
import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';
import { fill } from '@/lib/hive-i18n';
import { HiveShell, RichTitle, useMe, useT } from '@/components/hive/HiveShell';
import { Empty, ErrorNote, Loading, Notice, useConfirm } from '@/components/hive/ui';

export type Gathering = {
  id: string; title: string; type: string; description: string; starts_on: string; starts_at: string; place: string;
  places_left: number; my_status: '' | 'confirmed' | 'waitlist'; waitlist_position: number; started: boolean;
};

type Category = 'mixed' | 'women-only' | 'other';
type Filter = 'all' | Exclude<Category, 'other'>;

/** Legacy catalogue labels remain visible as "Other" until admins classify them. */
export function gatheringCategory(type: string): Category {
  const value = type.toLowerCase().trim().replace(/[ _]/g, '-');
  if (value === 'mixed' || value === 'mixed-gender' || value === 'mixed-gathering') return 'mixed';
  if (value === 'women-only' || value === 'women' || value === 'womenonly') return 'women-only';
  return 'other';
}

function GatheringCard({ g, onChanged }: { g: Gathering; onChanged: (message: string, tone?: 'ok' | 'err') => void }) {
  const { t, lang } = useT();
  const [busy, setBusy] = useState(false);
  const [eligible, setEligible] = useState(false);
  const [confirm, dialog] = useConfirm();
  const category = gatheringCategory(g.type);
  const categoryLabel = category === 'mixed' ? t.categoryMixed : category === 'women-only' ? t.categoryWomen : t.categoryOther;
  async function keep() {
    setBusy(true);
    try {
      const r = await api<{ status: string }>(`/gatherings/${g.id}/keep`, { method: 'POST', body: JSON.stringify({ womenOnlyAcknowledged: category === 'women-only' && eligible }) });
      onChanged(r.status === 'waitlist' ? t.waitlistedOk : t.bookedOk);
    } catch (e) { onChanged(e instanceof Error ? e.message : t.loadError, 'err'); } finally { setBusy(false); }
  }
  async function cancel() {
    const ok = await confirm({ title: g.my_status === 'waitlist' ? t.leaveWaitlist : t.cancelPlace, body: fill(t.cancelAsk, { title: g.title }), confirmLabel: t.confirm, cancelLabel: t.close, danger: true });
    if (!ok) return;
    setBusy(true);
    try { await api(`/gatherings/${g.id}/cancel`, { method: 'POST' }); onChanged(t.cancelledOk); }
    catch (e) { onChanged(e instanceof Error ? e.message : t.loadError, 'err'); } finally { setBusy(false); }
  }
  const status = g.my_status === 'confirmed' ? t.statusConfirmed : g.my_status === 'waitlist' ? fill(t.statusWaitlist, { n: g.waitlist_position }) : '';
  return (
    <article className="hv-card hv-gathering-card" aria-busy={busy}>
      <div className="hv-gathering-meta">
        <span className="hv-pill">{categoryLabel}</span>
        <span className="hv-pill grey">{t.categoryFree}</span>
      </div>
      <h3>{g.title}</h3>
      <p className="hv-muted">{g.starts_on} · {g.starts_at.slice(0, 5)} · {g.place}</p>
      {g.description && <p className="hv-pre">{g.description}</p>}
      {status ? <p><span className={`hv-pill${g.my_status === 'waitlist' ? ' honey' : ''}`}>{status}</span></p>
        : <p className="hv-muted">{g.places_left > 0 ? fill(t.placesLeftN, { n: g.places_left }) : t.fullWaitlist}</p>}
      {g.started ? <p className="hv-muted">{t.started}</p> : (
        <div className="hv-actions">
          {g.my_status
            ? <button type="button" className="hv-btn ghost" disabled={busy} onClick={() => void cancel()}>{g.my_status === 'waitlist' ? t.leaveWaitlist : t.cancelPlace}</button>
            : <div className="hv-reservation-action">
                {category === 'women-only' && <label className="hv-check hv-eligibility-check"><input type="checkbox" checked={eligible} onChange={(event) => setEligible(event.target.checked)} /><span>{lang === 'ar' ? 'أؤكد أنني مؤهلة لحضور هذا اللقاء المخصّص للنساء فقط، وأفهم أن المضيف قد يتحقق من الالتزام بشروط اللقاء.' : 'I confirm I meet this women-only gathering’s eligibility and understand the host may check the gathering rules.'}</span></label>}
                <button type="button" className="hv-btn" disabled={busy || (category === 'women-only' && !eligible)} onClick={() => void keep()}>{g.places_left > 0 ? t.keepPlace : t.joinWaitlist}</button>
              </div>}
        </div>
      )}
      {dialog}
    </article>
  );
}

export default function GatheringsScreen() {
  const { data: me } = useMe();
  return <HiveShell member><GatheringsBody approved={!!me && (me.isAdmin || me.status === 'approved')} known={!!me} /></HiveShell>;
}

function GatheringsBody({ approved, known }: { approved: boolean; known: boolean }) {
  const { t, lang } = useT();
  const qc = useQueryClient();
  const [filter, setFilter] = useState<Filter>('all');
  const [notice, setNotice] = useState<{ message: string; tone: 'ok' | 'err' }>({ message: '', tone: 'ok' });
  const { data: items = [], isLoading, error, refetch } = useQuery({ queryKey: ['gatherings'], queryFn: () => api<Gathering[]>('/gatherings'), enabled: approved });
  const shown = items.filter((g) => filter === 'all' || gatheringCategory(g.type) === filter);
  function changed(message: string, tone: 'ok' | 'err' = 'ok') {
    setNotice({ message, tone });
    void qc.invalidateQueries({ queryKey: ['gatherings'] });
    void qc.invalidateQueries({ queryKey: ['my-gatherings'] });
  }
  return (
    <>
      <div className="hv-hero"><RichTitle text={t.gTitle} /><p>{t.gLead}</p></div>
      {!known ? <Loading label={t.loading} /> : !approved ? <div className="hv-card">{t.gAwaiting}</div> : (
        <div className="hv-stack">
          <div className="hv-filter-row" role="group" aria-label={lang === 'ar' ? 'تصفية اللقاءات' : 'Filter gatherings'}>
            {([
              ['all', t.categoryAll],
              ['mixed', t.categoryMixed],
              ['women-only', t.categoryWomen],
            ] as const).map(([value, label]) => (
              <button key={value} type="button" className="hv-filter-chip" aria-pressed={filter === value} onClick={() => setFilter(value)}>{label}</button>
            ))}
          </div>
          <p className="hv-field-hint hv-category-help">{t.categoryHelp}</p>
          <Notice message={notice.message} tone={notice.tone} />
          {isLoading && <Loading label={t.loading} />}
          {error && <ErrorNote message={t.loadError} onRetry={() => void refetch()} retryLabel={t.retry} />}
          {!isLoading && !error && shown.length === 0 && <Empty>{t.gNone}</Empty>}
          {shown.map((g) => <GatheringCard key={g.id} g={g} onChanged={changed} />)}
        </div>
      )}
    </>
  );
}
