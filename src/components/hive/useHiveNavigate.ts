'use client';
import { useRouter } from 'next/navigation';
import { useCallback } from 'react';
/** Adapts the previous navigate({to, replace}) calls to Next.js App Router. */
export function useHiveNavigate() {
  const router = useRouter();
  return useCallback(({ to, replace = false }: { to: string; replace?: boolean }) => {
    if (replace) router.replace(to);
    else router.push(to);
  }, [router]);
}
