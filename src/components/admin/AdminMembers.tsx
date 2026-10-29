'use client';
import { useState } from 'react';
import Image from 'next/image';
import { ErrorNote, Loading, Empty, Notice, Pager } from '@/components/hive/ui';
import { SearchBox, useAdminAction, usePaged } from './shared';

type Member = { id: string; first_name: string; email: string; status: string; is_admin: boolean; email_verified: boolean; created_at: string; has_photo?: boolean };

export function AdminMembers() {
  const [page, setPage] = useState(1), [q, setQ] = useState(''), [status, setStatus] = useState('');
  const { data, isLoading, error, refetch } = usePaged<Member>('members', '/admin/members', { page, q, status });
  const { run, busy, message } = useAdminAction();
  return (
    <section className="hv-admin-members" aria-labelledby="members-h">
      <div className="hv-admin-section-head">
        <div>
          <h2 id="members-h">Membership approvals</h2>
          <p>Find members, review requests and manage access in one place.</p>
        </div>
        {data && !error && <span className="hv-admin-results" aria-live="polite">{data.total} {data.total === 1 ? 'result' : 'results'}</span>}
      </div>
      <div className="hv-toolbar hv-admin-toolbar">
        <div className="hv-admin-search">
          <SearchBox label="Search name or email" value={q} onChange={(v) => { setQ(v); setPage(1); }} />
        </div>
        <label className="hv-admin-filter">Status
          <select value={status} onChange={(e) => { setStatus(e.target.value); setPage(1); }}>
            <option value="">All members</option>
            <option value="pending">Pending</option>
            <option value="approved">Approved</option>
            <option value="declined">Declined</option>
          </select>
        </label>
      </div>
      <Notice message={message.text} tone={message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load members." onRetry={() => void refetch()} />}
      {data?.items.length === 0 && <Empty>No members match.</Empty>}
      <div className="hv-admin-member-list" role="list" aria-label="Member accounts">
        {data?.items.map((m) => (
          <div className="hv-admin-member-row" role="listitem" key={m.id}>
            <div className="hv-admin-member-identity">
              {m.has_photo
                ? <Image unoptimized width={44} height={44} className="hv-avatar hv-admin-member-avatar" alt={`Profile photo of ${m.first_name}`} src={`/api/backend/admin/members/${encodeURIComponent(m.id)}/photo`} />
                : <span className="hv-admin-member-initial" aria-hidden="true">{m.first_name.trim().charAt(0).toUpperCase() || 'H'}</span>}
              <div className="hv-admin-member-copy">
                <div className="hv-admin-member-name">
                  <strong>{m.first_name}</strong>
                  {m.is_admin && <span className="hv-pill">Admin</span>}
                  {!m.email_verified && <span className="hv-pill grey">Email not confirmed</span>}
                  {!m.is_admin && <span className={`hv-pill ${m.status === 'pending' ? 'honey' : m.status === 'declined' ? 'grey' : ''}`}>{m.status}</span>}
                </div>
                <span className="hv-admin-member-email">{m.email}</span>
              </div>
            </div>
            {m.is_admin
              ? <span className="hv-admin-cli-note">Managed with the admin CLI</span>
              : <label className="hv-admin-member-status">Status
                  <select aria-label={`Status for ${m.email}`} value={m.status} disabled={busy}
                    onChange={(e) => void run(`/admin/members/${m.id}`, 'PATCH', { status: e.target.value }, `Status for ${m.first_name} saved`)}>
                    <option value="pending">Pending</option>
                    <option value="approved">Approved</option>
                    <option value="declined">Declined</option>
                  </select>
                </label>}
          </div>
        ))}
      </div>
      {data && <Pager page={page} hasMore={data.has_more} total={data.total} onPage={setPage} />}
    </section>
  );
}
