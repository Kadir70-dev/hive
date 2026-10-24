'use client';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/hive-api';
import { HiveShell, useT } from '@/components/hive/HiveShell';
import { Empty, ErrorNote, Loading } from '@/components/hive/ui';

type Announcement = { id: string; title: string; body: string; published_at: string };
type Partner = { id: string; name: string; description: string; logo_url: string; website_url: string };
type Page<T> = { items: T[]; page: number; has_more: boolean };

function Announcements() {
  const { t } = useT();
  const q = useInfiniteQuery({
    queryKey: ['announcements'],
    initialPageParam: 1,
    queryFn: ({ pageParam }) => api<Page<Announcement>>(`/announcements?page=${pageParam}`),
    getNextPageParam: (last) => (last.has_more ? last.page + 1 : undefined),
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <div className="hv-stack">
      {q.isLoading && <Loading label={t.loading} />}
      {q.error && <ErrorNote message={t.loadError} onRetry={() => void q.refetch()} retryLabel={t.retry} />}
      {!q.isLoading && !q.error && items.length === 0 && <Empty>{t.annNone}</Empty>}
      {items.map((a) => (
        <article className="hv-card" key={a.id}>
          <h3>{a.title}</h3>
          <p className="hv-muted"><time dateTime={a.published_at}>{a.published_at.slice(0, 10)}</time></p>
          <p className="hv-pre">{a.body}</p>
        </article>
      ))}
      {q.hasNextPage && <button className="hv-btn ghost" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>{q.isFetchingNextPage ? t.loading : t.loadMore}</button>}
    </div>
  );
}

function Partners() {
  const { t } = useT();
  const { data = [], isLoading, error, refetch } = useQuery({ queryKey: ['partners'], queryFn: () => api<Partner[]>('/partners') });
  return (
    <div className="hv-stack">
      {isLoading && <Loading label={t.loading} />}
      {error && <ErrorNote message={t.loadError} onRetry={() => void refetch()} retryLabel={t.retry} />}
      {!isLoading && !error && data.length === 0 && <Empty>{t.partnersNone}</Empty>}
      {data.map((p) => (
        <article className="hv-card" key={p.id}>
          <h3>{p.name}</h3>
          {p.description && <p className="hv-pre">{p.description}</p>}
          {p.website_url.startsWith('https://') && <a href={p.website_url} target="_blank" rel="noopener noreferrer">{t.partnerSite}</a>}
        </article>
      ))}
    </div>
  );
}

export default function CommunityListScreen({ kind }: { kind: 'announcements' | 'partners' }) {
  return <HiveShell member><CommunityBody kind={kind} /></HiveShell>;
}

function CommunityBody({ kind }: { kind: 'announcements' | 'partners' }) {
  const { t } = useT();
  return (
    <>
      <div className="hv-hero"><h1>{kind === 'partners' ? t.partnersTitle : t.annTitle}</h1></div>
      {kind === 'partners' ? <Partners /> : <Announcements />}
    </>
  );
}
