'use client';
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';
import { useT } from '@/components/hive/HiveShell';
import { Empty, ErrorNote, Loading } from '@/components/hive/ui';

type Campaign = { source: string; medium: string; campaign: string; started: number; verified: number };
type Report = { items: Campaign[]; tracked_started: number; tracked_verified: number };

export function AdminCampaigns() {
  const { lang } = useT(), ar = lang === 'ar';
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['admin', 'campaigns'],
    queryFn: () => api<Report>('/admin/campaigns'),
  });
  return (
    <section aria-labelledby="campaigns-h">
      <h2 id="campaigns-h">{ar ? 'مصادر التسجيل' : 'Registration campaigns'}</h2>
      <p className="hv-muted">{ar ? 'أرقام مجمّعة من الموقع فقط، دون أسماء أو بيانات اتصال. يُحسب التسجيل المكتمل بعد تأكيد البريد الإلكتروني.' : 'First-party aggregate counts only: no member identities or contact details. A completed signup means the email address was verified.'}</p>
      {isLoading && <Loading label={ar ? 'جار التحميل…' : 'Loading campaign results…'} />}
      {error && <ErrorNote message={ar ? 'تعذّر تحميل النتائج.' : 'Unable to load campaigns.'} onRetry={() => void refetch()} />}
      {data && (
        <>
          <div className="hv-campaign-totals">
            <div><span className="hv-eyebrow">{ar ? 'طلبات مسجلة المصدر' : 'Attributed signups'}</span><strong>{data.tracked_started}</strong></div>
            <div><span className="hv-eyebrow">{ar ? 'بريد مؤكّد' : 'Email verified'}</span><strong>{data.tracked_verified}</strong></div>
          </div>
          {data.items.length === 0 ? <Empty>{ar ? 'لا توجد تسجيلات مرتبطة بحملات حتى الآن.' : 'No campaign-attributed registrations yet.'}</Empty> : (
            <div className="hv-table-scroll" role="region" aria-label={ar ? 'تقرير الحملات' : 'Campaign source report'} tabIndex={0}>
              <table className="hv-campaign-table">
                <thead><tr><th scope="col">{ar ? 'المصدر' : 'Source'}</th><th scope="col">{ar ? 'الوسيط' : 'Medium'}</th><th scope="col">{ar ? 'الحملة' : 'Campaign'}</th><th scope="col">{ar ? 'الطلبات' : 'Started'}</th><th scope="col">{ar ? 'المكتمل' : 'Verified'}</th></tr></thead>
                <tbody>{data.items.map((c, i) => <tr key={c.source + c.medium + c.campaign + i}>
                  <td>{c.source || '—'}</td><td>{c.medium || '—'}</td><td>{c.campaign || '—'}</td><td>{c.started}</td><td><strong>{c.verified}</strong></td>
                </tr>)}</tbody>
              </table>
            </div>
          )}
        </>
      )}
    </section>
  );
}
