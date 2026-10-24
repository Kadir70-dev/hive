'use client';
import { useState, type FormEvent } from 'react';
import { Empty, ErrorNote, Loading, Notice, Pager, useConfirm } from '@/components/hive/ui';
import { isHttpsURL, SearchBox, useAdminAction, usePaged } from './shared';

type Partner = { id: string; name: string; description: string; logo_url: string; website_url: string; published: boolean };

function PartnerForm({ initial, onSubmit, submitLabel, onCancel }: { initial?: Partner; onSubmit: (v: Record<string, unknown>) => Promise<boolean>; submitLabel: string; onCancel?: () => void }) {
  const [error, setError] = useState(''), [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    const v = { name: String(f.get('name') || ''), description: String(f.get('description') || ''), websiteURL: String(f.get('websiteURL') || '').trim(), logoURL: String(f.get('logoURL') || '').trim(), published: f.get('published') === 'on' };
    if (!isHttpsURL(v.websiteURL) || !isHttpsURL(v.logoURL)) { setError('Links must be full https:// addresses.'); return; }
    setError(''); setBusy(true);
    const ok = await onSubmit(v);
    setBusy(false);
    if (ok && !initial) form.reset();
  }
  return (
    <form className="hv-stack" onSubmit={(e) => void submit(e)} noValidate>
      <label>Name<input name="name" required maxLength={200} defaultValue={initial?.name} /></label>
      <label>Description<textarea name="description" maxLength={5000} defaultValue={initial?.description} /></label>
      <label>Website (https://)<input name="websiteURL" type="url" inputMode="url" placeholder="https://" defaultValue={initial?.website_url} /></label>
      <label>Logo URL (https://, optional)<input name="logoURL" type="url" inputMode="url" placeholder="https://" defaultValue={initial?.logo_url} /></label>
      <label className="hv-check"><input type="checkbox" name="published" defaultChecked={initial ? initial.published : true} /> <span>Visible to members</span></label>
      {error && <p className="hv-msg err" role="alert">{error}</p>}
      <div className="hv-actions">
        <button className="hv-btn" disabled={busy}>{busy ? 'Saving…' : submitLabel}</button>
        {onCancel && <button type="button" className="hv-btn ghost" onClick={onCancel}>Cancel</button>}
      </div>
    </form>
  );
}

export function AdminPartners() {
  const [page, setPage] = useState(1), [q, setQ] = useState(''), [editing, setEditing] = useState('');
  const { data, isLoading, error, refetch } = usePaged<Partner>('partners', '/admin/partners', { page, q });
  const action = useAdminAction();
  const [confirm, dialog] = useConfirm();
  async function remove(p: Partner) {
    if (await confirm({ title: `Remove ${p.name}?`, body: 'The partner disappears from the members’ directory.', confirmLabel: 'Remove', cancelLabel: 'Keep', danger: true }))
      await action.run(`/admin/partners/${p.id}`, 'DELETE', undefined, 'Partner removed');
  }
  return (
    <section aria-labelledby="partners-h">
      <h2 id="partners-h">Community partners</h2>
      <details className="hv-card">
        <summary>Add a partner</summary>
        <PartnerForm submitLabel="Add partner" onSubmit={async (v) => !!(await action.run('/admin/partners', 'POST', v, 'Partner added'))} />
      </details>
      <div className="hv-toolbar"><SearchBox label="Search partners" value={q} onChange={(v) => { setQ(v); setPage(1); }} /></div>
      <Notice message={action.message.text} tone={action.message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load partners." onRetry={() => void refetch()} />}
      {data?.items.length === 0 && <Empty>No partners.</Empty>}
      {data?.items.map((p) => (
        <div className="hv-item" key={p.id}>
          <div className="hv-item-head"><h3>{p.name}</h3><span className={`hv-pill${p.published ? '' : ' grey'}`}>{p.published ? 'visible' : 'hidden'}</span></div>
          {editing === p.id
            ? <PartnerForm initial={p} submitLabel="Save changes" onCancel={() => setEditing('')}
                onSubmit={async (v) => { const ok = !!(await action.run(`/admin/partners/${p.id}`, 'PUT', v, 'Partner saved')); if (ok) setEditing(''); return ok; }} />
            : <>{p.description && <p className="hv-pre">{p.description}</p>}{p.website_url && <span className="hv-muted">{p.website_url}</span>}</>}
          <div className="hv-actions">
            <button className="hv-btn ghost sm" onClick={() => setEditing(editing === p.id ? '' : p.id)}>Edit</button>
            <button className="hv-btn danger sm" disabled={action.busy} onClick={() => void remove(p)}>Remove</button>
          </div>
        </div>
      ))}
      {data && <Pager page={page} hasMore={data.has_more} total={data.total} onPage={setPage} />}
      {dialog}
    </section>
  );
}
