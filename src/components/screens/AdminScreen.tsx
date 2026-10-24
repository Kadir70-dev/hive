'use client';
import { useState } from 'react';
import { HiveShell, useMe } from '@/components/hive/HiveShell';
import { MfaGate } from '@/components/screens/MfaGate';
import { Loading } from '@/components/hive/ui';
import { AdminMembers } from '@/components/admin/AdminMembers';
import { AdminGatherings } from '@/components/admin/AdminGatherings';
import { AdminAnnouncements } from '@/components/admin/AdminAnnouncements';
import { AdminPartners } from '@/components/admin/AdminPartners';
import { AdminReports } from '@/components/admin/AdminReports';

const TABS = [
  ['members', 'Members', AdminMembers],
  ['gatherings', 'Gatherings', AdminGatherings],
  ['announcements', 'Announcements', AdminAnnouncements],
  ['partners', 'Partners', AdminPartners],
  ['reports', 'Reports', AdminReports],
] as const;

function AdminTools() {
  const [tab, setTab] = useState<(typeof TABS)[number][0]>('members');
  const Panel = TABS.find(([id]) => id === tab)![2];
  return (
    <div className="hv-card">
      <div className="hv-tabs" role="tablist" aria-label="Admin sections">
        {TABS.map(([id, label]) => (
          <button key={id} type="button" role="tab" id={`tab-${id}`} aria-selected={tab === id} aria-controls={`panel-${id}`} className="hv-link" onClick={() => setTab(id)}>{label}</button>
        ))}
      </div>
      <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}><Panel /></div>
    </div>
  );
}

/** Admin tools render only after the MFA gate passes (the API enforces the same rule). */
export default function AdminScreen() {
  const { data: me, isLoading } = useMe();
  return (
    <HiveShell member>
      <div className="hv-hero"><h1>Administration</h1><p>Private member approvals, gatherings, check-ins and community content.</p></div>
      {isLoading ? <Loading /> : !me?.isAdmin ? <div className="hv-card">Admin access required.</div> : <MfaGate me={me}><AdminTools /></MfaGate>}
    </HiveShell>
  );
}
