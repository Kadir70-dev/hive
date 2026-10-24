'use client';
import Link from "next/link";
import { HiveShell, RichTitle, useMe, useT } from "@/components/hive/HiveShell";


function MembersHome() {
  const { t } = useT();
  const { data: me, isLoading } = useMe();
  if (isLoading || !me) return <div className="hv-hero"><p>…</p></div>;

  if (me.status === "approved") {
    return (
      <div className="hv-hero">
        <span className="hv-eyebrow">{t.authEyebrow}</span>
        <RichTitle text={t.welcomeTitle} values={{ name: me.first_name }} />
        <p>{t.welcomeText}</p>
        <div className="hv-actions" style={{ justifyContent: "center", marginTop: 28 }}>
          <Link href="/gatherings" className="hv-btn">{t.seeGatherings}</Link>
          <Link href="/profile" className="hv-btn ghost">{t.editProfile}</Link>
        </div>
      </div>
    );
  }

  const declined = me.status === "declined";
  return (
    <div className="hv-hero">
      <span className="hv-pill honey">{declined ? t.declined : t.pending}</span>
      <RichTitle text={declined ? t.declinedTitle : t.pendingTitle} />
      <p>{declined ? t.declinedText : t.pendingText}</p>
      {!declined && (
        <div className="hv-actions" style={{ justifyContent: "center", marginTop: 28 }}>
          <Link href="/guidelines" className="hv-btn ghost">{t.guidelines}</Link>
        </div>
      )}
    </div>
  );
}


export default function MembersScreen() {
  return <HiveShell member><MembersHome /></HiveShell>;
}
