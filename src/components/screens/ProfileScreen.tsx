'use client';
import { useState, type FormEvent } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { HiveShell, useMe, useT, type Me } from '@/components/hive/HiveShell';
import { api, send } from '@/lib/hive-api';
import { fill } from '@/lib/hive-i18n';
import { Empty, ErrorNote, Loading, Notice } from '@/components/hive/ui';
import { ProfilePhoto } from '@/components/screens/ProfilePhoto';

type MyGathering = { id: string; title: string; starts_on: string; starts_at: string; place: string; status: 'confirmed' | 'waitlist'; waitlist_position?: number; past: boolean; attended: boolean };

function MyGatherings() {
  const { t } = useT();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: ['my-gatherings'], queryFn: () => api<MyGathering[]>('/me/gatherings') });
  return (
    <div className="hv-card">
      <h2>{t.myGatherings}</h2>
      {isLoading && <Loading label={t.loading} />}
      {error && <ErrorNote message={t.loadError} onRetry={() => void refetch()} retryLabel={t.retry} />}
      {data?.length === 0 && <Empty>{t.noneJoined}</Empty>}
      {data?.map((g) => (
        <div className="hv-item" key={g.id}>
          <div className="hv-item-head">
            <h3>{g.title}</h3>
            {g.past
              ? <span className="hv-pill grey">{g.attended ? t.attendedLabel : t.pastLabel}</span>
              : <span className={`hv-pill${g.status === 'waitlist' ? ' honey' : ''}`}>{g.status === 'waitlist' ? fill(t.statusWaitlist, { n: g.waitlist_position ?? '' }) : t.statusConfirmed}</span>}
          </div>
          <span className="hv-muted">{g.starts_on} · {g.starts_at.slice(0, 5)} · {g.place}</span>
        </div>
      ))}
    </div>
  );
}

/** Form state is seeded from `me` once; remounting (key) re-seeds after a reload of `me`. */
function ProfileForm({ me }: { me: Me }) {
  const { t } = useT();
  const qc = useQueryClient(), router = useRouter();
  const [notice, setNotice] = useState<{ message: string; tone: 'ok' | 'err' }>({ message: '', tone: 'ok' });
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [name, setName] = useState(me.first_name), [display, setDisplay] = useState(me.display_name || ''), [phone, setPhone] = useState(me.phone || '');
  const [area, setArea] = useState(me.area || ''), [interests, setInterests] = useState((me.interests || []).join(', ')), [updates, setUpdates] = useState(!!me.email_updates);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await send('/me', { firstName: name, displayName: display, phone, area, interests: interests.split(',').map((s) => s.trim()).filter(Boolean), emailUpdates: updates }, 'PATCH');
      await qc.invalidateQueries({ queryKey: ['me'] });
      setNotice({ message: t.saved, tone: 'ok' });
    } catch (err) { setNotice({ message: err instanceof Error ? err.message : t.loadError, tone: 'err' }); } finally { setBusy(false); }
  }
  async function del(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    try {
      await send('/me', { password: String(new FormData(e.currentTarget).get('password') || '') }, 'DELETE');
      qc.clear();
      router.replace('/auth');
    } catch (err) { setNotice({ message: err instanceof Error ? err.message : 'Unable to delete account', tone: 'err' }); setBusy(false); }
  }
  return (
    <div className="hv-card">
      <p>Membership status: <strong>{me.status}</strong></p>
      <p>Email (private): {me.email}</p>
      <p className="hv-field-hint">{t.privateNote}</p>
      <form className="hv-stack" onSubmit={(e) => void save(e)}>
        <label>{t.firstName}<input required maxLength={60} value={name} onChange={(e) => setName(e.target.value)} autoComplete="given-name" /></label>
        <label>Display name<input maxLength={60} value={display} onChange={(e) => setDisplay(e.target.value)} /></label>
        <label>{t.phone}
          <input type="tel" inputMode="tel" maxLength={20} pattern="\+?[0-9 ()\-]{7,20}" title="7 to 20 digits, e.g. +971 50 123 4567" value={phone} onChange={(e) => setPhone(e.target.value)} autoComplete="tel" />
        </label>
        <label>{t.area}<input maxLength={80} value={area} onChange={(e) => setArea(e.target.value)} /></label>
        <label>Interests (comma separated, up to 10)<input value={interests} onChange={(e) => setInterests(e.target.value)} /></label>
        <label className="hv-check">
          <input type="checkbox" checked={updates} onChange={(e) => setUpdates(e.target.checked)} aria-describedby="updates-hint" />
          <span>{t.emailUpdatesLabel}<br /><span id="updates-hint" className="hv-field-hint">{t.emailUpdatesHint}</span></span>
        </label>
        <Notice message={notice.message} tone={notice.tone} />
        <button className="hv-btn" disabled={busy}>{busy ? t.loading : t.save}</button>
      </form>
      <hr />
      {!deleting ? <button className="hv-btn ghost" onClick={() => setDeleting(true)}>{t.deleteTitle}</button> : (
        <form className="hv-stack" onSubmit={(e) => void del(e)} aria-label={t.deleteTitle}>
          <p>{t.deleteText} Enter your password to confirm.</p>
          <label>Password<input name="password" type="password" required autoComplete="current-password" /></label>
          <div className="hv-actions">
            <button className="hv-btn danger" disabled={busy}>Delete permanently</button>
            <button type="button" className="hv-btn ghost" onClick={() => setDeleting(false)}>{t.keep}</button>
          </div>
        </form>
      )}
    </div>
  );
}

function ProfileBody() {
  const { t } = useT();
  const { data: me, isLoading } = useMe();
  return (
    <>
      <div className="hv-hero"><h1>{t.profile}</h1><p>Your personal details are visible to admins only.</p></div>
      {isLoading && <Loading label={t.loading} />}
      {me && <div className="hv-stack"><ProfilePhoto me={me} /><MyGatherings /><ProfileForm key={me.id} me={me} /></div>}
    </>
  );
}

export default function ProfileScreen() {
  return <HiveShell member><ProfileBody /></HiveShell>;
}
