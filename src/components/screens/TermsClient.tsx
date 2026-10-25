'use client';
import Link from 'next/link';
import { useLang } from '@/lib/hive-i18n';
import { arabicTerms, type PolicySection } from '@/lib/hive-policy-ar';

export default function TermsClient({ english }: { english: PolicySection[] }) {
  const { lang, setLang } = useLang();
  const ar = lang === 'ar';
  const sections = ar ? arabicTerms : english;
  return (
    <main className="hv hv-wrap hv-policy" lang={lang} dir={ar ? 'rtl' : 'ltr'}>
      <article className="hv-card" style={{ marginTop: 60 }}>
        <div className="hv-policy-top">
          <Link href="/">{ar ? 'العودة للرئيسية' : 'Back to home'}</Link>
          <button className="hv-btn ghost sm" type="button" onClick={() => setLang(ar ? 'en' : 'ar')}>{ar ? 'English' : 'العربية'}</button>
        </div>
        <h1>{ar ? 'شروط العضوية' : 'Terms of Membership'}</h1>
        <p role="note" className="hv-policy-draft"><strong>{ar ? 'مسودة للمراجعة — لم تعتمدها The Hive Society أو مستشاروها القانونيون بعد. ليست استشارة قانونية.' : 'Draft for review — not yet approved by The Hive Society or its legal advisers. Not legal advice.'}</strong> {ar ? 'آخر تحديث: أكتوبر ٢٠٢٦ (مسودة).' : 'Last updated: October 2026 (draft).'}</p>
        {sections.map((s) => (
          <section key={s.title} className="hv-policy-section">
            <h2>{s.title}</h2>
            {s.body.map((p) => <p key={p}>{p}</p>)}
          </section>
        ))}
        <p>
          {ar ? 'انظر أيضاً:' : 'See also the'}{' '}
          <Link href="/guidelines">{ar ? 'إرشادات المجتمع' : 'Community Guidelines'}</Link>{' · '}
          <Link href="/privacy">{ar ? 'سياسة الخصوصية' : 'Privacy Policy'}</Link>{' · '}
          <Link href="/">{ar ? 'العودة للرئيسية' : 'Back to home'}</Link>
        </p>
      </article>
    </main>
  );
}
