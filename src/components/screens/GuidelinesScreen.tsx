'use client';
import { HiveShell, RichTitle, useT } from "@/components/hive/HiveShell";


function Guidelines() {
  const { t } = useT();
  return (
    <>
      <div className="hv-hero">
        <span className="hv-eyebrow">{t.glEyebrow}</span>
        <RichTitle text={t.glTitle} />
        <p>{t.glLead}</p>
      </div>
      <div className="hv-card">
        <ol className="hv-list">
          {t.gl.map(([title, body], i) => (
            <li key={title}>
              <span className="hv-num">{String(i + 1).padStart(2, "0")}</span>
              <div>
                <h2 style={{ marginBottom: 4 }}>{title}</h2>
                <p>{body}</p>
              </div>
            </li>
          ))}
        </ol>
      </div>
      <p className="hv-muted" style={{ textAlign: "center", marginTop: 24 }}>{t.glClose}</p>
    </>
  );
}


export default function GuidelinesScreen() {
  return <HiveShell><Guidelines /></HiveShell>;
}
