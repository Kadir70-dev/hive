'use client';
import { useRef, useState, type KeyboardEvent } from 'react';
import { HiveShell, useMe, useT } from '@/components/hive/HiveShell';
import { MfaGate } from '@/components/screens/MfaGate';
import { Loading } from '@/components/hive/ui';
import { AdminMembers } from '@/components/admin/AdminMembers';
import { AdminGatherings } from '@/components/admin/AdminGatherings';
import { AdminAnnouncements } from '@/components/admin/AdminAnnouncements';
import { AdminPartners } from '@/components/admin/AdminPartners';
import { AdminReports } from '@/components/admin/AdminReports';
import { AdminSafetyConflicts } from '@/components/admin/AdminSafetyConflicts';

const TABS = [
  ['members', 'Members', AdminMembers],
  ['gatherings', 'Gatherings', AdminGatherings],
  ['announcements', 'Announcements', AdminAnnouncements],
  ['partners', 'Partners', AdminPartners],
  ['reports', 'Reports', AdminReports],
  ['safety-conflicts', 'Safety conflicts', AdminSafetyConflicts],
] as const;

export function AdminTools() {
  const { lang } = useT();
  const [tab, setTab] = useState<(typeof TABS)[number][0]>('members');
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
  function moveTab(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = TABS.length - 1;
    let next: number;
    switch (e.key) {
      case 'ArrowRight': next = (index + (lang === 'ar' ? last : 1)) % TABS.length; break;
      case 'ArrowLeft': next = (index + (lang === 'ar' ? 1 : last)) % TABS.length; break;
      case 'ArrowDown': next = (index + 1) % TABS.length; break;
      case 'ArrowUp': next = (index + last) % TABS.length; break;
      case 'Home': next = 0; break;
      case 'End': next = last; break;
      default: return;
    }
    e.preventDefault();
    setTab(TABS[next]![0]);
    tabRefs.current[next]?.focus();
  }
  const Panel = TABS.find(([id]) => id === tab)![2];
  const arLabels: Record<(typeof TABS)[number][0], string> = { members: 'الأعضاء', gatherings: 'اللقاءات', announcements: 'الإعلانات', partners: 'الشركاء', reports: 'البلاغات', 'safety-conflicts': 'تعارضات السلامة' };
  return (
    <div className="hv-card">
      <div className="hv-tabs" role="tablist" aria-label={lang === "ar" ? "أقسام الإدارة" : "Admin sections"}>
        {TABS.map(([id, label], index) => (
          <button key={id} type="button" role="tab" id={`tab-${id}`}
            ref={(element) => { tabRefs.current[index] = element; }}
            tabIndex={tab === id ? 0 : -1}
            aria-selected={tab === id} aria-controls={`panel-${id}`}
            className="hv-link" onClick={() => setTab(id)}
            onKeyDown={(event) => moveTab(event, index)}>{lang === "ar" ? arLabels[id] : label}</button>
        ))}
      </div>
      <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}><Panel /></div>
    </div>
  );
}

/** Admin tools render only after the MFA gate passes (the API enforces the same rule). */
function AdminBody() {
  const { data: me, isLoading } = useMe();
  const { lang } = useT();
  return (
    <>
      <div className="hv-hero"><h1>{lang === "ar" ? "الإدارة" : "Administration"}</h1><p>{lang === "ar" ? "إدارة العضويات واللقاءات وتسجيل الحضور والمحتوى المجتمعي بخصوصية." : "Private member approvals, gatherings, check-ins and community content."}</p></div>
      {isLoading ? <Loading /> : !me?.isAdmin ? <div className="hv-card">{lang === "ar" ? "يتطلب هذا القسم صلاحية المشرف." : "Admin access required."}</div> : <MfaGate me={me}><AdminTools /></MfaGate>}
    </>
  );
}
export default function AdminScreen() { return <HiveShell member><AdminBody /></HiveShell>; }
