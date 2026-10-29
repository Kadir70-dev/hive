'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { HiveShell, useMe, useT, type Me } from '@/components/hive/HiveShell';
import { api, send } from '@/lib/hive-api';
import { AREAS, fill } from '@/lib/hive-i18n';
import { isAbuDhabiArea, normalizeUAEPhone } from '@/lib/uae-member-details';
import { Empty, ErrorNote, Loading, Notice } from '@/components/hive/ui';
import { ProfilePhoto } from '@/components/screens/ProfilePhoto';

type MyGathering = { id: string; title: string; starts_on: string; starts_at: string; place: string; status: 'confirmed' | 'waitlist'; waitlist_position?: number; past: boolean; attended: boolean };

function MyGatherings({ status }: { status: Me['status'] }) {
  const { t } = useT();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: ['my-gatherings'], queryFn: () => api<MyGathering[]>('/me/gatherings') });
  return (
    <section className="hv-card hv-profile-gatherings" aria-labelledby="my-gatherings-heading">
      <h2 id="my-gatherings-heading">{t.myGatherings}</h2>
      {isLoading && <Loading label={t.loading} />}
      {error && <ErrorNote message={t.loadError} onRetry={() => void refetch()} retryLabel={t.retry} />}
      {data?.length === 0 && (
        <div className="hv-profile-empty">
          <Empty>{t.noneJoined}</Empty>
          {status === 'approved' ? <Link className="hv-btn ghost sm" href="/gatherings">{t.seeGatherings}</Link> :
            status === 'pending' ? <p className="hv-field-hint">{t.gAwaiting}</p> : null}
        </div>
      )}
      {data?.map((g) => (
        <div className="hv-item" key={g.id}>
          <div className="hv-item-head">
            <h3>{g.title}</h3>
            {g.past
              ? <span className="hv-pill grey">{g.attended ? t.attendedLabel : t.pastLabel}</span>
              : <span className={'hv-pill' + (g.status === 'waitlist' ? ' honey' : '')}>{g.status === 'waitlist' ? fill(t.statusWaitlist, { n: g.waitlist_position ?? '' }) : t.statusConfirmed}</span>}
          </div>
          <span className="hv-muted">{g.starts_on} · {g.starts_at.slice(0, 5)} · {g.place}</span>
        </div>
      ))}
    </section>
  );
}

/** Form state is seeded from me once; remounting after a reload re-seeds it. */
function ProfileForm({ me }: { me: Me }) {
  const { t, lang } = useT();
  const qc = useQueryClient(), router = useRouter();
  const [notice, setNotice] = useState<{ message: string; tone: 'ok' | 'err' }>({ message: '', tone: 'ok' });
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [name, setName] = useState(me.first_name), [display, setDisplay] = useState(me.display_name || ''), [phone, setPhone] = useState(me.phone || '');
  const [area, setArea] = useState(me.area || ''), [interests, setInterests] = useState((me.interests || []).join(', ')), [updates, setUpdates] = useState(!!me.email_updates);

  async function save(e: FormEvent) {
    e.preventDefault();
    const normalizedPhone = normalizeUAEPhone(phone);
    if (!normalizedPhone) {
      setNotice({ message: lang === 'ar' ? 'رقم الهاتف الإماراتي مطلوب، مثال: ‎+971 50 123 4567' : 'UAE mobile is required (e.g. +971 50 123 4567).', tone: 'err' });
      return;
    }
    if (!isAbuDhabiArea(area)) {
      setNotice({ message: lang === 'ar' ? 'اختر منطقة في أبوظبي.' : 'Choose an Abu Dhabi area.', tone: 'err' });
      return;
    }
    setBusy(true);
    try {
      await send('/me', { firstName: name, displayName: display, phone: normalizedPhone, area, interests: interests.split(',').map((s) => s.trim()).filter(Boolean), emailUpdates: updates }, 'PATCH');
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
    <section className="hv-card hv-profile-details" aria-labelledby="profile-details-heading">
      <div className="hv-profile-section-head">
        <h2 id="profile-details-heading">{t.profileDetails}</h2>
        <p className="hv-field-hint">{t.privateNote}</p>
      </div>
      <form className="hv-stack" onSubmit={(e) => void save(e)}>
        <div className="hv-profile-fields">
          <label>{t.firstName}<input required maxLength={60} value={name} onChange={(e) => setName(e.target.value)} autoComplete="given-name" /></label>
          <label>{lang === 'ar' ? 'اسم العرض' : 'Display name'}<input maxLength={60} value={display} onChange={(e) => setDisplay(e.target.value)} /></label>
          <label>{lang === 'ar' ? 'رقم الهاتف الإماراتي' : 'UAE mobile number'} *
            <input type="tel" inputMode="tel" required maxLength={24} value={phone} onChange={(e) => setPhone(e.target.value)} autoComplete="tel" placeholder="+971 50 123 4567" dir="ltr" />
            <span className="hv-field-hint">{lang === 'ar' ? 'رقم إماراتي فقط، مثال: ‎+971 50 123 4567' : 'UAE mobile numbers only, e.g. +971 50 123 4567.'}</span>
          </label>
          <label>{lang === 'ar' ? 'المنطقة في أبوظبي' : 'Area in Abu Dhabi'} *
            <select required value={area} onChange={(e) => setArea(e.target.value)}>
              <option value="" disabled>{lang === 'ar' ? 'اختر منطقتك' : 'Choose your area'}</option>
              {area && !isAbuDhabiArea(area) && <option value={area} disabled>{area} — {lang === 'ar' ? 'اختر منطقة في أبوظبي' : 'Please select an Abu Dhabi area'}</option>}
              {AREAS.map((item) => <option key={item} value={item}>{item === 'Other' ? (lang === 'ar' ? 'منطقة أخرى في أبوظبي' : 'Other area in Abu Dhabi') : item}</option>)}
            </select>
          </label>
          <label className="hv-profile-field-full">{lang === 'ar' ? 'الاهتمامات (مفصولة بفواصل، حتى ١٠)' : 'Interests (comma separated, up to 10)'}<input value={interests} onChange={(e) => setInterests(e.target.value)} /></label>
        </div>
        <label className="hv-check">
          <input type="checkbox" checked={updates} onChange={(e) => setUpdates(e.target.checked)} aria-describedby="updates-hint" />
          <span>{t.emailUpdatesLabel}<span id="updates-hint" className="hv-field-hint hv-profile-update-hint">{t.emailUpdatesHint}</span></span>
        </label>
        <Notice message={notice.message} tone={notice.tone} />
        <div className="hv-profile-save"><button type="submit" className="hv-btn" disabled={busy}>{busy ? t.loading : t.save}</button></div>
      </form>
      <div className="hv-profile-account">
        <h3>{t.profileAccount}</h3>
        {!deleting ? <button type="button" className="hv-btn danger sm" onClick={() => setDeleting(true)}>{t.deleteTitle}</button> : (
          <form className="hv-stack" onSubmit={(e) => void del(e)} aria-label={t.deleteTitle}>
            <p>{t.deleteText} {lang === 'ar' ? 'أدخل كلمة المرور للتأكيد.' : 'Enter your password to confirm.'}</p>
            <label>{lang === 'ar' ? 'كلمة المرور' : 'Password'}<input name="password" type="password" required autoComplete="current-password" /></label>
            <div className="hv-actions">
              <button className="hv-btn danger" disabled={busy}>{lang === 'ar' ? 'حذف نهائي' : 'Delete permanently'}</button>
              <button type="button" className="hv-btn ghost" onClick={() => setDeleting(false)}>{t.keep}</button>
            </div>
          </form>
        )}
      </div>
    </section>
  );
}

function ProfileBody() {
  const { t } = useT();
  const { data: me, isLoading } = useMe();
  return (
    <div className="hv-profile">
      <div className="hv-profile-intro">
        <span className="hv-eyebrow">{t.pEyebrow}</span>
        <h1>{t.profile}</h1>
        <p>{t.profileIntro}</p>
      </div>
      {isLoading && <Loading label={t.loading} />}
      {me && (
        <>
          <section className="hv-card hv-profile-summary" aria-label={t.profileOverview}>
            <div className="hv-profile-summary-main">
              <span className="hv-avatar" aria-hidden="true">{me.first_name.slice(0, 1).toUpperCase()}</span>
              <div className="hv-profile-identity">
                <p className="hv-profile-kicker">{t.profileOverview}</p>
                <h2>{me.display_name || me.first_name}</h2>
                <p className="hv-profile-email">{t.profileEmailPrivate}: <span dir="ltr">{me.email}</span></p>
              </div>
            </div>
            <div className="hv-profile-status">
              <span className={'hv-pill' + (me.status === 'pending' ? ' honey' : me.status === 'declined' ? ' grey' : '')}>
                {me.status === 'approved' ? t.approved : me.status === 'pending' ? t.pending : t.declined}
              </span>
              {me.status === 'pending' && <p className="hv-field-hint">{t.gAwaiting}</p>}
            </div>
          </section>
          <div className="hv-profile-grid">
            <div className="hv-profile-main">
              <ProfileForm key={me.id} me={me} />
            </div>
            <aside className="hv-profile-aside">
              <MyGatherings status={me.status} />
              {me.photos_enabled && <ProfilePhoto me={me} />}
            </aside>
          </div>
        </>
      )}
    </div>
  );
}

export default function ProfileScreen() {
  return <HiveShell member><ProfileBody /></HiveShell>;
}
