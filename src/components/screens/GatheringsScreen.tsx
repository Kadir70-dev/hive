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

function GatheringCard({ g, onChanged }: { g: Gathering; onChanged: (message: string, tone?: 'ok' | 'err') => void }) {
  const { t } = useT();
  const [busy, setBusy] = useState(false);
  const [confirm, dialog] = useConfirm();
  async function keep() {
    setBusy(true);
    try {
      const r = await api<{ status: string }>(`/gatherings/${g.id}/keep`, { method: 'POST' });
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
    <article className="hv-card" aria-busy={busy}>
      <span className="hv-pill">{g.type}</span>
      <h3>{g.title}</h3>
      <p>{g.starts_on} · {g.starts_at.slice(0, 5)} · {g.place}</p>
      {g.description && <p className="hv-pre">{g.description}</p>}
      {status ? <p><span className={`hv-pill${g.my_status === 'waitlist' ? ' honey' : ''}`}>{status}</span></p>
        : <p className="hv-muted">{g.places_left > 0 ? fill(t.placesLeftN, { n: g.places_left }) : t.fullWaitlist}</p>}
      {g.started ? <p className="hv-muted">{t.started}</p> : (
        <div className="hv-actions">
          {g.my_status
            ? <button className="hv-btn ghost" disabled={busy} onClick={() => void cancel()}>{g.my_status === 'waitlist' ? t.leaveWaitlist : t.cancelPlace}</button>
            : <button className="hv-btn" disabled={busy} onClick={() => void keep()}>{g.places_left > 0 ? t.keepPlace : t.joinWaitlist}</button>}
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
  const { t } = useT();
  const qc = useQueryClient();
  const [notice, setNotice] = useState<{ message: string; tone: 'ok' | 'err' }>({ message: '', tone: 'ok' });
  const { data: items = [], isLoading, error, refetch } = useQuery({ queryKey: ['gatherings'], queryFn: () => api<Gathering[]>('/gatherings'), enabled: approved });
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
          <Notice message={notice.message} tone={notice.tone} />
          {isLoading && <Loading label={t.loading} />}
          {error && <ErrorNote message={t.loadError} onRetry={() => void refetch()} retryLabel={t.retry} />}
          {!isLoading && !error && items.length === 0 && <Empty>{t.gNone}</Empty>}
          {items.map((g) => <GatheringCard key={g.id} g={g} onChanged={changed} />)}
        </div>
      )}
    </>
  );
}
