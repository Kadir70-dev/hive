'use client';
import { useState, type FormEvent } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError, send } from '@/lib/hive-api';
import { HiveShell, useT } from '@/components/hive/HiveShell';
import { AREAS, INTERESTS } from '@/lib/hive-i18n';
import { isAbuDhabiArea, normalizeUAEPhone } from '@/lib/uae-member-details';
import { captureCampaign, savedCampaign } from '@/lib/campaign-attribution';

const COPY = {
  en: {
    joinTitle: 'Join The Hive Society', welcomeTitle: 'Welcome back',
    lead: 'An Abu Dhabi community for men and women of all nationalities. Discover free, curated gatherings — with dedicated women-only experiences.',
    signIn: 'Log in', signUp: 'Sign up', firstName: 'First name', displayName: 'Display name (optional)',
    phone: 'UAE mobile (optional)', phoneHint: 'UAE numbers only, e.g. +971 50 123 4567.', phoneError: 'Enter a valid UAE mobile number or leave this blank.', area: 'Area in Abu Dhabi', areaHint: 'Choose your Abu Dhabi neighbourhood', areaError: 'Choose an Abu Dhabi area to continue.', email: 'Email', password: 'Password', passwordHint: 'At least 12 characters for a new account.',
    agree: 'I agree to the', terms: 'Terms of Membership', and: 'and', privacy: 'Privacy Policy',
    create: 'Create account', wait: 'Please wait…', forgot: 'Forgot your password?',
    checkTitle: 'Check your email', checkIntro: 'We sent a confirmation link to',
    checkOutro: 'Open it to confirm your address, then log in. The link expires in 24 hours.',
    nothing: 'Nothing arrived? Check your spam folder, or', request: 'request a new link', resend: 'Send a new confirmation link',
    resendOk: 'If that address needs confirming, a new link is on its way.',
    unable: 'Unable to continue', unableSend: 'Unable to send',
    private: 'Your contact details are private. Applications are reviewed before membership approval.',
    interests: 'Interests (optional)', photoLater: 'You can add a private profile photo after joining.',
    interestNames: ['Wellness', 'Majlis', 'Culture & heritage', 'Nature & walks', 'Social gatherings'],
  },
  ar: {
    joinTitle: 'انضم إلى The Hive Society', welcomeTitle: 'أهلاً بعودتك',
    lead: 'مجتمع في أبوظبي للنساء والرجال من جميع الجنسيات. اكتشف لقاءات مجانية منسّقة، مع تجارب مخصّصة للنساء.',
    signIn: 'تسجيل الدخول', signUp: 'إنشاء حساب', firstName: 'الاسم الأول', displayName: 'اسم العرض (اختياري)',
    phone: 'رقم هاتف إماراتي (اختياري)', phoneHint: 'رقم إماراتي فقط، مثال: ‎+971 50 123 4567', phoneError: 'أدخل رقم هاتف إماراتي صحيحاً أو اترك الحقل فارغاً.', area: 'المنطقة في أبوظبي', areaHint: 'اختر منطقتك في أبوظبي', areaError: 'اختر منطقة في أبوظبي للمتابعة.', email: 'البريد الإلكتروني', password: 'كلمة المرور', passwordHint: '١٢ حرفاً على الأقل للحساب الجديد.',
    agree: 'أوافق على', terms: 'شروط العضوية', and: 'و', privacy: 'سياسة الخصوصية',
    create: 'إنشاء حساب', wait: 'يرجى الانتظار…', forgot: 'هل نسيت كلمة المرور؟',
    checkTitle: 'تحقّق من بريدك', checkIntro: 'أرسلنا رابط تأكيد إلى',
    checkOutro: 'افتح الرابط لتأكيد بريدك ثم سجّل الدخول. تنتهي صلاحية الرابط بعد ٢٤ ساعة.',
    nothing: 'لم يصلك البريد؟ تحقّق من الرسائل غير المرغوب فيها أو', request: 'اطلب رابطاً جديداً', resend: 'إرسال رابط تأكيد جديد',
    resendOk: 'إذا كان البريد يحتاج إلى تأكيد فسيصل إليه رابط جديد.',
    unable: 'تعذّر المتابعة', unableSend: 'تعذّر الإرسال',
    private: 'بيانات الاتصال خاصة. تتم مراجعة الطلبات قبل الموافقة على العضوية.',
    interests: 'الاهتمامات (اختياري)', photoLater: 'يمكنك إضافة صورة خاصة لملفك بعد الانضمام.',
    interestNames: ['العافية', 'المجلس', 'الثقافة والتراث', 'الطبيعة والتجوال', 'اللقاءات الاجتماعية'],
  },
} as const;

function AuthBody() {
  const { lang } = useT();
  const t = COPY[lang];
  const router = useRouter(), qc = useQueryClient(), params = useSearchParams();
  const [signup, setSignup] = useState(() => params.get('mode') === 'signup');
  const [busy, setBusy] = useState(false), [error, setError] = useState('');
  const [sentTo, setSentTo] = useState(''), [unverified, setUnverified] = useState(''), [notice, setNotice] = useState('');

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true); setError(''); setNotice(''); setUnverified('');
    const f = new FormData(e.currentTarget);
    const email = String(f.get('email') || '').trim();
    const password = String(f.get('password') || '');
    try {
      if (signup) {
        const phone = normalizeUAEPhone(String(f.get('phone') || ''));
        const area = String(f.get('area') || '');
        if (phone === null) { setError(t.phoneError); return; }
        if (!isAbuDhabiArea(area)) { setError(t.areaError); return; }
        let attribution = null;
        try { attribution = captureCampaign(window.location.search, window.sessionStorage) || savedCampaign(window.sessionStorage); } catch { /* storage unavailable */ }
        await send('/signup', {
          email, password, firstName: String(f.get('first_name') || '').trim(),
          displayName: String(f.get('display_name') || '').trim(), phone,
          interests: f.getAll('interest').map(String), area,
          ...(attribution || {}),
        });
        setSentTo(email);
        return;
      }
      const res = await send<{ mfa_required?: boolean }>('/login', { email, password });
      await qc.invalidateQueries({ queryKey: ['me'] });
      router.replace(res.mfa_required ? '/admin' : '/members');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'email_unverified') { setUnverified(email); setError(err.message); }
      else setError(err instanceof Error ? err.message : t.unable);
    } finally { setBusy(false); }
  }

  async function resend() {
    setBusy(true); setError('');
    try { await send('/verify-email/resend', { email: unverified }); setNotice(t.resendOk); }
    catch (err) { setError(err instanceof Error ? err.message : t.unableSend); }
    finally { setBusy(false); }
  }

  if (sentTo) return (
    <div className="hv-auth-layout">
      <div className="hv-hero"><h1>{t.checkTitle}</h1><p>{t.checkIntro} <strong dir="ltr">{sentTo}</strong>. {t.checkOutro}</p></div>
      <div className="hv-card hv-auth-card">
        <p>{t.nothing} <button type="button" className="hv-link" onClick={() => { setUnverified(sentTo); setSentTo(''); setSignup(false); }}>{t.request}</button>.</p>
      </div>
    </div>
  );

  return (
    <div className="hv-auth-layout">
      <div className="hv-hero"><h1>{signup ? t.joinTitle : t.welcomeTitle}</h1><p>{t.lead}</p></div>
      <div className="hv-card hv-auth-card">
        <div className="hv-auth-tabs" role="group" aria-label={lang === 'ar' ? 'نوع الحساب' : 'Account access'}>
          <button className="hv-auth-tab" type="button" aria-pressed={!signup} onClick={() => { setSignup(false); setError(''); }}>{t.signIn}</button>
          <button className="hv-auth-tab" type="button" aria-pressed={signup} onClick={() => { setSignup(true); setError(''); }}>{t.signUp}</button>
        </div>
        <p className="hv-field-hint hv-auth-privacy">{t.private}</p>
        <form className="hv-stack" onSubmit={(e) => void submit(e)}>
          {signup && <>
            <label>{t.firstName} *<input name="first_name" required maxLength={60} autoComplete="given-name" /></label>
            <label>{t.displayName}<input name="display_name" maxLength={60} autoComplete="nickname" /></label>
            <label>{t.phone}
              <span className="hv-field-hint">{t.phoneHint}</span>
              <input name="phone" type="tel" inputMode="tel" maxLength={24} autoComplete="tel" placeholder="+971 50 123 4567" dir="ltr" />
            </label>
            <label>{t.area} *
              <select name="area" required defaultValue="">
                <option value="" disabled>{t.areaHint}</option>
                {AREAS.map((area) => <option key={area} value={area}>{area === 'Other' ? (lang === 'ar' ? 'منطقة أخرى في أبوظبي' : 'Other area in Abu Dhabi') : area}</option>)}
              </select>
            </label>
            <fieldset className="hv-interest-fieldset"><legend>{t.interests}</legend><div className="hv-interest-options">{INTERESTS.map((item, index) => <label className="hv-interest-chip" key={item}><input type="checkbox" name="interest" value={item} /><span>{t.interestNames[index]}</span></label>)}</div></fieldset>
            <p className="hv-field-hint">{t.photoLater}</p>
          </>}
          <label>{t.email} *<input name="email" required type="email" inputMode="email" autoComplete="email" defaultValue={unverified} /></label>
          <label>{t.password} *<input name="password" required type="password" minLength={signup ? 12 : 1} autoComplete={signup ? 'new-password' : 'current-password'} /></label>
          {signup && <>
            <p className="hv-field-hint">{t.passwordHint}</p>
            <label className="hv-check"><input type="checkbox" required /> <span>{t.agree} <Link href="/terms">{t.terms}</Link> {t.and} <Link href="/privacy">{t.privacy}</Link>.</span></label>
          </>}
          {error && <p role="alert" className="hv-msg err">{error}</p>}
          {notice && <p role="status" className="hv-msg">{notice}</p>}
          {unverified && <button type="button" className="hv-btn ghost" disabled={busy} onClick={() => void resend()}>{t.resend}</button>}
          <button type="submit" className="hv-btn hv-auth-submit" disabled={busy}>{busy ? t.wait : signup ? t.create : t.signIn}</button>
          {!signup && <p className="hv-auth-forgot"><Link href="/reset-password">{t.forgot}</Link></p>}
        </form>
      </div>
    </div>
  );
}

export default function AuthScreen() {
  return <HiveShell><AuthBody /></HiveShell>;
}
