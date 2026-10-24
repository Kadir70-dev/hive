'use client';
import { useState, type FormEvent } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';
import { Empty, ErrorNote, Loading, Notice, useConfirm } from '@/components/hive/ui';
import { GatheringEditor } from '@/components/screens/GatheringEditor';
import { SearchBox, useAdminAction } from './shared';

type Gathering = { id: string; title: string; type: string; description: string; starts_on: string; starts_at: string; place: string;
  capacity: number; confirmed: number; waitlisted: number; attended: number; started: boolean };
type Attendee = { user_id: string; first_name: string; email: string; status: string; attended: boolean; waitlist_position?: number; block_conflict: boolean };

function Roster({ g, onAction }: { g: Gathering; onAction: ReturnType<typeof useAdminAction> }) {
  const { data, isLoading, error, refetch } = useQuery({ queryKey: ['admin', 'attendees', g.id], queryFn: () => api<Attendee[]>(`/admin/gatherings/${g.id}/attendees`) });
  return (
    <div className="hv-card" aria-label={`Who's coming to ${g.title}`}>
      <h3>Who&apos;s coming · {g.confirmed}/{g.capacity} confirmed · {g.waitlisted} waiting · {g.attended} checked in</h3>
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load the roster." onRetry={() => void refetch()} />}
      {data?.length === 0 && <Empty>No one yet.</Empty>}
      {data?.map((a) => (
        <div className="hv-row" key={a.user_id}>
          <span>
            {a.first_name} · {a.email}{' '}
            <span className={`hv-pill${a.status === 'waitlist' ? ' honey' : ''}`}>{a.status === 'waitlist' ? `waitlist #${a.waitlist_position}` : 'confirmed'}</span>
            {a.block_conflict && <span className="hv-pill honey" title="A safety block exists between this member and someone else on this list"> safety conflict</span>}
          </span>
          {a.status === 'confirmed'
            ? <label className="hv-check"><input type="checkbox" checked={a.attended} disabled={onAction.busy}
                onChange={(e) => void onAction.run('/admin/checkin', 'POST', { gatheringID: g.id, userID: a.user_id, attended: e.target.checked }, `${a.first_name} ${e.target.checked ? 'checked in' : 'unchecked'}`)} /> Attended</label>
            : <span className="hv-muted">Check-in opens once promoted</span>}
        </div>
      ))}
    </div>
  );
}

export function AdminGatherings() {
  const { data = [], isLoading, error, refetch } = useQuery({ queryKey: ['admin', 'gatherings'], queryFn: () => api<Gathering[]>('/admin/gatherings') });
  const action = useAdminAction();
  const [confirm, dialog] = useConfirm();
  const [q, setQ] = useState(''), [editing, setEditing] = useState(''), [roster, setRoster] = useState('');
  const shown = data.filter((g) => !q || `${g.title} ${g.place} ${g.type}`.toLowerCase().includes(q.toLowerCase()));

  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = Object.fromEntries(new FormData(form).entries());
    if (await action.run('/admin/gatherings', 'POST', { ...f, capacity: Number(f.capacity) }, 'Gathering added')) form.reset();
  }
  async function remove(g: Gathering) {
    const n = g.confirmed + g.waitlisted;
    if (await confirm({ title: `Cancel “${g.title}”?`, body: n ? `${n} member${n === 1 ? '' : 's'} with a place or on the waitlist will be emailed that it is cancelled.` : 'No one has booked yet.', confirmLabel: 'Cancel gathering', cancelLabel: 'Keep it', danger: true }))
      await action.run(`/admin/gatherings/${g.id}`, 'DELETE', undefined, 'Gathering cancelled');
  }
  return (
    <section aria-labelledby="gatherings-h">
      <h2 id="gatherings-h">Gatherings</h2>
      <details className="hv-card">
        <summary>Add a gathering</summary>
        <form className="hv-stack" onSubmit={(e) => void create(e)}>
          <label>Title<input name="title" required maxLength={200} /></label>
          <label>Category<input name="type" defaultValue="community" maxLength={60} /></label>
          <label>Description<textarea name="description" maxLength={5000} /></label>
          <label>Date<input type="date" name="startsOn" required /></label>
          <label>Time (Abu Dhabi)<input type="time" name="startsAt" required /></label>
          <label>Venue<input name="place" required maxLength={300} /></label>
          <label>Capacity<input type="number" name="capacity" min={1} max={10000} required /></label>
          <button className="hv-btn" disabled={action.busy}>Add gathering</button>
        </form>
      </details>
      <div className="hv-toolbar"><SearchBox label="Search gatherings" value={q} onChange={setQ} /></div>
      <Notice message={action.message.text} tone={action.message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load gatherings." onRetry={() => void refetch()} />}
      {!isLoading && shown.length === 0 && <Empty>No gatherings.</Empty>}
      {shown.map((g) => (
        <div className="hv-item" key={g.id}>
          <div className="hv-item-head">
            <h3>{g.title}</h3>
            <span className="hv-muted">{g.starts_on} · {g.starts_at.slice(0, 5)} · {g.place}{g.started ? ' · past' : ''}</span>
          </div>
          <span className="hv-muted">{g.confirmed}/{g.capacity} confirmed · {g.waitlisted} waiting · {g.attended} attended</span>
          <div className="hv-actions">
            <button className="hv-btn ghost sm" aria-expanded={roster === g.id} onClick={() => setRoster(roster === g.id ? '' : g.id)}>Roster & check-in</button>
            <button className="hv-btn ghost sm" aria-expanded={editing === g.id} onClick={() => setEditing(editing === g.id ? '' : g.id)}>Edit</button>
            <button className="hv-btn danger sm" disabled={action.busy} onClick={() => void remove(g)}>Cancel gathering</button>
          </div>
          {editing === g.id && <GatheringEditor g={g} onClose={() => setEditing('')} onSave={async (payload) => !!(await action.run(`/admin/gatherings/${g.id}`, 'PUT', payload, 'Gathering updated'))} />}
          {roster === g.id && <Roster g={g} onAction={action} />}
        </div>
      ))}
      {dialog}
    </section>
  );
}
