'use client';
import { useRef, useState, type KeyboardEvent } from 'react';
import Link from 'next/link';
import { HiveShell, useMe, useT } from '@/components/hive/HiveShell';
import { MfaGate } from '@/components/screens/MfaGate';
import { Loading } from '@/components/hive/ui';
import { AdminMembers } from '@/components/admin/AdminMembers';
import { AdminGatherings } from '@/components/admin/AdminGatherings';
import { AdminAnnouncements } from '@/components/admin/AdminAnnouncements';
import { AdminPartners } from '@/components/admin/AdminPartners';
import { AdminReports } from '@/components/admin/AdminReports';
import { AdminSafetyConflicts } from '@/components/admin/AdminSafetyConflicts';
import { AdminCampaigns } from '@/components/admin/AdminCampaigns';

const TABS = [
  ['members', 'Members', AdminMembers],
  ['gatherings', 'Gatherings', AdminGatherings],
  ['announcements', 'Announcements', AdminAnnouncements],
  ['partners', 'Partners', AdminPartners],
  ['campaigns', 'Signups', AdminCampaigns],
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
  const arLabels: Record<(typeof TABS)[number][0], string> = { members: 'الأعضاء', gatherings: 'اللقاءات', announcements: 'الإعلانات', partners: 'الشركاء', campaigns: 'مصادر التسجيل', reports: 'البلاغات', 'safety-conflicts': 'تعارضات السلامة' };
  return (
    <div className="hv-card hv-admin-console">
      <div className="hv-admin-console-topline">
        <span>{lang === 'ar' ? 'مساحة عمل خاصة' : 'PRIVATE WORKSPACE'}</span>
        <Link href="/studio" className="hv-admin-studio-link">{lang === 'ar' ? 'استوديو الموقع' : 'Website Studio'} <span aria-hidden="true">↗</span></Link>
      </div>
      <div className="hv-tabs hv-admin-tabs" role="tablist" aria-label={lang === "ar" ? "أقسام الإدارة" : "Admin sections"}>
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
    <div className="hv-admin-page">
      <header className="hv-admin-hero">
        <span className="hv-admin-eyebrow">{lang === 'ar' ? 'ذا هايف سوسايتي / الإدارة' : 'THE HIVE SOCIETY / MANAGEMENT'}</span>
        <h1>{lang === 'ar' ? 'الإدارة' : 'Administration'}</h1>
        <p>{lang === 'ar' ? 'مكان واحد لإدارة الأعضاء واللقاءات والمحتوى المجتمعي بخصوصية.' : 'A quieter place to manage members, gatherings and community content.'}</p>
      </header>
      {isLoading ? <Loading /> : !me?.isAdmin ? <div className="hv-card">{lang === 'ar' ? 'يتطلب هذا القسم صلاحية المشرف.' : 'Admin access required.'}</div> : <MfaGate me={me}><AdminTools /></MfaGate>}
    </div>
  );
}
export default function AdminScreen() { return <HiveShell member><AdminBody /></HiveShell>; }
