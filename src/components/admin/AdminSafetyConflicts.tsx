'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';
import { Empty, ErrorNote, Loading, Notice, useConfirm } from '@/components/hive/ui';
import { useAdminAction } from '@/components/admin/shared';

type Participant = { id: string; first_name: string; status: string };
type Conflict = {
  gathering_id: string; gathering_title: string; starts_on: string; starts_at: string; report_id: string;
  reporter: Participant; reported: Participant;
};
type Decision = 'acknowledged' | 'released_reporter' | 'released_reported';

function ConflictRow({ conflict, action }: { conflict: Conflict; action: ReturnType<typeof useAdminAction> }) {
  const [decision, setDecision] = useState<Decision>('acknowledged');
  const [note, setNote] = useState('');
  const [confirm, dialog] = useConfirm();
  const release = decision !== 'acknowledged';
  async function resolve() {
    const target = decision === 'released_reporter' ? conflict.reporter.first_name : conflict.reported.first_name;
    const yes = await confirm({
      title: release ? 'Release a reserved place?' : 'Record safety decision?',
      body: release
        ? `Release ${target}'s place from ${conflict.gathering_title}? The member receives a neutral email, and the earliest eligible waitlisted member is promoted.`
        : 'Both reservations remain unchanged. Record the safety decision for authorized admins.',
      confirmLabel: release ? 'Release place' : 'Record decision',
      cancelLabel: 'Keep reviewing', danger: release,
    });
    if (!yes) return;
    await action.run('/admin/safety-conflicts/resolve', 'POST', {
      gatheringID: conflict.gathering_id, reportID: conflict.report_id, decision, note,
    }, 'Safety decision recorded');
  }
  return (
    <article className="hv-item">
      <div className="hv-item-head">
        <h3>{conflict.gathering_title}</h3>
        <span className="hv-muted">{conflict.starts_on} · {conflict.starts_at.slice(0, 5)}</span>
      </div>
      <p className="hv-muted">Confidential: {conflict.reporter.first_name} ({conflict.reporter.status}) and {conflict.reported.first_name} ({conflict.reported.status}). Never share their identities with other members.</p>
      <label>Decision
        <select value={decision} onChange={(e) => setDecision(e.target.value as Decision)}>
          <option value="acknowledged">Keep both reservations, record a note</option>
          <option value="released_reporter">Release reporter&apos;s place</option>
          <option value="released_reported">Release reported member&apos;s place</option>
        </select>
      </label>
      <label>Internal note (admin only)
        <textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={1000} placeholder="Reason and any follow-up arrangements" />
      </label>
      <div className="hv-actions"><button type="button" disabled={action.busy} className={release ? 'hv-btn danger' : 'hv-btn'} onClick={() => void resolve()}>Save decision</button></div>
      {dialog}
    </article>
  );
}

export function AdminSafetyConflicts() {
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['admin', 'safety-conflicts'],
    queryFn: () => api<Conflict[]>('/admin/safety-conflicts'),
  });
  const action = useAdminAction();
  return (
    <section aria-labelledby="safety-conflicts-heading">
      <h2 id="safety-conflicts-heading">Existing booking conflicts</h2>
      <p className="hv-muted">When a safety block is added after two people book, an admin must explicitly decide what to do. Nothing is automatically cancelled. These details are visible only after administrator two-factor verification.</p>
      <Notice message={action.message.text} tone={action.message.tone} />
      {isLoading && <Loading />}
      {error && <ErrorNote message="Could not load safety conflicts." onRetry={() => void refetch()} />}
      {!isLoading && data?.length === 0 && <Empty>No unresolved conflicts for upcoming gatherings.</Empty>}
      {data?.map((conflict) => <ConflictRow key={`${conflict.gathering_id}:${conflict.report_id}`} conflict={conflict} action={action} />)}
    </section>
  );
}
