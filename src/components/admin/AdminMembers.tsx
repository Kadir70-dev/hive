'use client';
import { useState } from 'react';
import { ErrorNote, Loading, Empty, Notice, Pager } from '@/components/hive/ui';
import { SearchBox, useAdminAction, usePaged } from './shared';

type Member = { id: string; first_name: string; email: string; status: string; is_admin: boolean; email_verified: boolean; created_at: string };

export function AdminMembers() {
  const [page, setPage] = useState(1), [q, setQ] = useState(''), [status, setStatus] = useState('');
  const { data, isLoading, error, refetch } = usePaged<Member>('members', '/admin/members', { page, q, status });
  const { run, busy, message } = useAdminAction();
  return (
    <section aria-labelledby="members-h">
      <h2 id="members-h">Membership approvals</h2>
      <div className="hv-toolbar">
        <SearchBox label="Search name or email" value={q} onChange={(v) => { setQ(v); setPage(1); }} />
        <label>Status<select value={status} onChange={(e) => { setStatus(e.target.value); setPage(1); }}>
          <option value="">All</option><option value="pending">Pending</option><option value="approved">Approved</option><option value="declined">Declined</option>
        </select></label>
      </div>
      <Notice message={message.text} tone={message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load members." onRetry={() => void refetch()} />}
      {data?.items.length === 0 && <Empty>No members match.</Empty>}
      {data?.items.map((m) => (
        <div className="hv-row" key={m.id}>
          <span>
            <strong>{m.first_name}</strong> · {m.email}
            {!m.email_verified && <span className="hv-pill grey"> email not confirmed</span>}
            {m.is_admin && <span className="hv-pill"> admin</span>}
          </span>
          {m.is_admin ? <span className="hv-muted">Managed with the admin CLI</span> : (
            <label className="hv-actions">Status
              <select aria-label={`Status for ${m.email}`} value={m.status} disabled={busy}
                onChange={(e) => void run(`/admin/members/${m.id}`, 'PATCH', { status: e.target.value }, `Status for ${m.first_name} saved`)}>
                <option value="pending">pending</option><option value="approved">approved</option><option value="declined">declined</option>
              </select>
            </label>
          )}
        </div>
      ))}
      {data && <Pager page={page} hasMore={data.has_more} total={data.total} onPage={setPage} />}
    </section>
  );
}
