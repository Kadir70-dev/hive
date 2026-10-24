'use client';
import { useState, type FormEvent } from 'react';

export type EditableGathering = { id: string; title: string; type: string; description: string; starts_on: string; starts_at: string; place: string; capacity: number };

/**
 * Inline editor for one gathering. Capacity can be raised (the waitlist is
 * promoted in order) or lowered down to the number of confirmed places; the
 * API refuses anything lower. Members are emailed when date, time or venue change.
 */
export function GatheringEditor({ g, onSave, onClose }: { g: EditableGathering; onSave: (payload: Record<string, unknown>) => Promise<boolean>; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const f = Object.fromEntries(new FormData(e.currentTarget).entries());
    const ok = await onSave({ ...f, capacity: Number(f.capacity) });
    setBusy(false);
    if (ok) onClose();
  }
  return (
    <form className="hv-stack hv-card" onSubmit={submit} aria-label={`Edit ${g.title}`}>
      <label>Title<input name="title" defaultValue={g.title} required maxLength={200} /></label>
      <label>Category<input name="type" defaultValue={g.type} maxLength={60} /></label>
      <label>Description<textarea name="description" defaultValue={g.description} maxLength={5000} /></label>
      <label>Date<input type="date" name="startsOn" defaultValue={g.starts_on} required /></label>
      <label>Time<input type="time" name="startsAt" defaultValue={g.starts_at.slice(0, 5)} required /></label>
      <label>Venue<input name="place" defaultValue={g.place} required maxLength={300} /></label>
      <label>Capacity<input type="number" name="capacity" min={1} max={10000} defaultValue={g.capacity} required /></label>
      <div className="hv-actions">
        <button className="hv-btn" disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button>
        <button type="button" className="hv-btn ghost" onClick={onClose}>Cancel</button>
      </div>
    </form>
  );
}
