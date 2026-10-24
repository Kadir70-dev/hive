'use client';
import { useState, type FormEvent } from 'react';
import { Empty, ErrorNote, Loading, Notice, Pager, useConfirm } from '@/components/hive/ui';
import { SearchBox, useAdminAction, usePaged } from './shared';

type Announcement = { id: string; title: string; body: string; published: boolean; created_at: string; published_at: string; emailed_count: number };

function AnnouncementForm({ initial, onSubmit, submitLabel, onCancel }: { initial?: Announcement; onSubmit: (v: { title: string; body: string; published: boolean }) => Promise<boolean>; submitLabel: string; onCancel?: () => void }) {
  const [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    setBusy(true);
    const ok = await onSubmit({ title: String(f.get('title') || ''), body: String(f.get('body') || ''), published: f.get('published') === 'on' });
    setBusy(false);
    if (ok && !initial) form.reset();
  }
  return (
    <form className="hv-stack" onSubmit={(e) => void submit(e)}>
      <label>Title<input name="title" required maxLength={200} defaultValue={initial?.title} /></label>
      <label>Text<textarea name="body" required maxLength={10000} rows={6} defaultValue={initial?.body} /></label>
      <label className="hv-check"><input type="checkbox" name="published" defaultChecked={initial ? initial.published : true} /> <span>Published (visible to approved members)</span></label>
      <div className="hv-actions">
        <button className="hv-btn" disabled={busy}>{busy ? 'Saving…' : submitLabel}</button>
        {onCancel && <button type="button" className="hv-btn ghost" onClick={onCancel}>Cancel</button>}
      </div>
    </form>
  );
}

export function AdminAnnouncements() {
  const [page, setPage] = useState(1), [q, setQ] = useState(''), [editing, setEditing] = useState('');
  const { data, isLoading, error, refetch } = usePaged<Announcement>('announcements', '/admin/announcements', { page, q });
  const action = useAdminAction();
  const [confirm, dialog] = useConfirm();

  async function email(a: Announcement) {
    if (!(await confirm({ title: 'Email this announcement?', body: 'It goes only to approved members who turned on email updates. Members who already received it are skipped, so repeating this never sends twice.', confirmLabel: 'Send emails', cancelLabel: 'Not now' }))) return;
    await action.run<{ queued: number }>(`/admin/announcements/${a.id}/email`, 'POST', undefined,
      (r) => (r.queued === 0 ? 'Everyone opted in has already received it' : `${r.queued} email${r.queued === 1 ? '' : 's'} queued`));
  }
  async function remove(a: Announcement) {
    if (await confirm({ title: `Delete “${a.title}”?`, body: 'Members will no longer see it. This cannot be undone.', confirmLabel: 'Delete', cancelLabel: 'Keep', danger: true }))
      await action.run(`/admin/announcements/${a.id}`, 'DELETE', undefined, 'Announcement deleted');
  }
  return (
    <section aria-labelledby="ann-h">
      <h2 id="ann-h">Announcements</h2>
      <details className="hv-card">
        <summary>New announcement</summary>
        <AnnouncementForm submitLabel="Create announcement" onSubmit={async (v) => !!(await action.run('/admin/announcements', 'POST', v, 'Announcement created'))} />
      </details>
      <div className="hv-toolbar"><SearchBox label="Search announcements" value={q} onChange={(v) => { setQ(v); setPage(1); }} /></div>
      <Notice message={action.message.text} tone={action.message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load announcements." onRetry={() => void refetch()} />}
      {data?.items.length === 0 && <Empty>No announcements.</Empty>}
      {data?.items.map((a) => (
        <div className="hv-item" key={a.id}>
          <div className="hv-item-head">
            <h3>{a.title}</h3>
            <span><span className={`hv-pill${a.published ? '' : ' grey'}`}>{a.published ? 'published' : 'draft'}</span> {a.emailed_count > 0 && <span className="hv-muted">emailed to {a.emailed_count}</span>}</span>
          </div>
          {editing === a.id
            ? <AnnouncementForm initial={a} submitLabel="Save changes" onCancel={() => setEditing('')}
                onSubmit={async (v) => { const ok = !!(await action.run(`/admin/announcements/${a.id}`, 'PUT', v, 'Announcement saved')); if (ok) setEditing(''); return ok; }} />
            : <p className="hv-pre">{a.body.length > 280 ? `${a.body.slice(0, 280)}…` : a.body}</p>}
          <div className="hv-actions">
            <button className="hv-btn ghost sm" onClick={() => setEditing(editing === a.id ? '' : a.id)}>Edit</button>
            <button className="hv-btn ghost sm" disabled={action.busy} onClick={() => void action.run(`/admin/announcements/${a.id}`, 'PUT', { title: a.title, body: a.body, published: !a.published }, a.published ? 'Unpublished' : 'Published')}>{a.published ? 'Unpublish' : 'Publish'}</button>
            <button className="hv-btn ghost sm" disabled={action.busy || !a.published} title={a.published ? undefined : 'Publish first'} onClick={() => void email(a)}>Email opted-in members</button>
            <button className="hv-btn danger sm" disabled={action.busy} onClick={() => void remove(a)}>Delete</button>
          </div>
        </div>
      ))}
      {data && <Pager page={page} hasMore={data.has_more} total={data.total} onPage={setPage} />}
      {dialog}
    </section>
  );
}
