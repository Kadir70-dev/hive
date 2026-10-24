'use client';
import { useState, type FormEvent } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';
import { Empty, ErrorNote, Loading, Notice, Pager, useConfirm } from '@/components/hive/ui';
import { useAdminAction, usePaged } from './shared';

type Report = { id: string; category: string; status: string; reason: string; subject_description: string; block_requested: boolean; resolution_note: string;
  created_at: string; reporter_id: string; reporter_name: string; reported_member_id: string; reported_member_name: string; gathering_id: string; gathering_title: string; block_applied: boolean };
type Person = { id: string; label: string };

const STATUSES = [['open', 'Open'], ['in_review', 'In review'], ['resolved', 'Resolved'], ['dismissed', 'Dismissed']] as const;

/** Candidates to link: the gathering roster when known, otherwise a member search. Never the reporter. */
function useCandidates(r: Report, q: string): Person[] {
  const roster = useQuery({ queryKey: ['admin', 'attendees', r.gathering_id], enabled: !!r.gathering_id,
    queryFn: () => api<{ user_id: string; first_name: string; email: string }[]>(`/admin/gatherings/${r.gathering_id}/attendees`) });
  const search = useQuery({ queryKey: ['admin', 'member-search', q], enabled: !r.gathering_id && q.length >= 2,
    queryFn: () => api<{ items: { id: string; first_name: string; email: string }[] }>(`/admin/members?q=${encodeURIComponent(q)}&page_size=10`) });
  const people: Person[] = r.gathering_id
    ? (roster.data ?? []).map((a) => ({ id: a.user_id, label: `${a.first_name} · ${a.email}` }))
    : (search.data?.items ?? []).map((m) => ({ id: m.id, label: `${m.first_name} · ${m.email}` }));
  return people.filter((p) => p.id !== r.reporter_id);
}

function ReportCard({ r, action }: { r: Report; action: ReturnType<typeof useAdminAction> }) {
  const [q, setQ] = useState('');
  const people = useCandidates(r, q);
  const [confirm, dialog] = useConfirm();
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    await action.run(`/admin/reports/${r.id}`, 'PATCH', { status: f.get('status'), resolutionNote: f.get('note'), reportedMemberID: f.get('member') || '' }, 'Report updated');
  }
  async function block() {
    if (await confirm({ title: `Keep ${r.reporter_name} and ${r.reported_member_name} apart?`, body: 'Neither will be able to book a gathering the other has booked. Neither is told who is involved. Existing bookings are not changed.', confirmLabel: 'Apply block', cancelLabel: 'Cancel' }))
      await action.run(`/admin/reports/${r.id}/block`, 'POST', undefined, 'Block applied');
  }
  return (
    <article className="hv-item" aria-label={`Report from ${r.reporter_name}`}>
      <div className="hv-item-head">
        <h3>{r.category}{r.gathering_title ? ` · ${r.gathering_title}` : ''}</h3>
        <span className="hv-pill">{r.status.replace('_', ' ')}</span>
      </div>
      <span className="hv-muted">From {r.reporter_name} · {r.created_at.slice(0, 16).replace('T', ' ')}{r.block_requested ? ' · asked not to be booked together' : ''}</span>
      {r.subject_description && <p><strong>Who:</strong> {r.subject_description}</p>}
      <p className="hv-pre">{r.reason}</p>
      <form className="hv-stack" onSubmit={(e) => void save(e)}>
        <label>Status<select name="status" defaultValue={r.status}>{STATUSES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}</select></label>
        <label>Internal resolution note (not shown to members)<textarea name="note" maxLength={2000} defaultValue={r.resolution_note} /></label>
        {!r.gathering_id && <label>Find the member concerned<input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="name or email" /></label>}
        <label>Member concerned
          <select name="member" defaultValue={r.reported_member_id}>
            <option value="">{r.reported_member_id ? `${r.reported_member_name} (current)` : 'Not linked'}</option>
            {people.filter((p) => p.id !== r.reported_member_id).map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          </select>
        </label>
        <div className="hv-actions">
          <button className="hv-btn" disabled={action.busy}>Save</button>
          {r.reported_member_id && !r.block_applied && <button type="button" className="hv-btn ghost" disabled={action.busy} onClick={() => void block()}>Apply block</button>}
          {r.block_applied && <button type="button" className="hv-btn ghost" disabled={action.busy} onClick={() => void action.run(`/admin/reports/${r.id}/block`, 'DELETE', undefined, 'Block lifted')}>Lift block</button>}
        </div>
      </form>
      {dialog}
    </article>
  );
}

export function AdminReports() {
  const [page, setPage] = useState(1), [status, setStatus] = useState('open');
  const { data, isLoading, error, refetch } = usePaged<Report>('reports', '/admin/reports', { page, status });
  const action = useAdminAction();
  return (
    <section aria-labelledby="reports-h">
      <h2 id="reports-h">Safety reports</h2>
      <p className="hv-muted">Report contents are visible to admins only. Members are never told who reported them.</p>
      <div className="hv-toolbar">
        <label>Show<select value={status} onChange={(e) => { setStatus(e.target.value); setPage(1); }}>
          <option value="">All</option>{STATUSES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select></label>
      </div>
      <Notice message={action.message.text} tone={action.message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load reports." onRetry={() => void refetch()} />}
      {data?.items.length === 0 && <Empty>No reports here.</Empty>}
      {data?.items.map((r) => <ReportCard key={r.id} r={r} action={action} />)}
      {data && <Pager page={page} hasMore={data.has_more} total={data.total} onPage={setPage} />}
    </section>
  );
}
