import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import AuthScreen from '@/components/screens/AuthScreen';

let search = '';
vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(search),
}));

function renderAuth(query: string) {
  search = query;
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 401 })));
  return render(<QueryClientProvider client={qc}><AuthScreen /></QueryClientProvider>);
}

describe('AuthScreen', () => {
  it('opens the sign-up form for /auth?mode=signup', () => {
    renderAuth('mode=signup');
    expect(screen.getByRole('heading', { name: 'Join The Hive Society' })).toBeInTheDocument();
    expect(screen.getByLabelText(/First name/)).toBeInTheDocument();
  });

  it('opens the log-in form by default', () => {
    renderAuth('');
    expect(screen.getByRole('heading', { name: 'Welcome back' })).toBeInTheDocument();
    expect(screen.queryByLabelText(/First name/)).not.toBeInTheDocument();
  });
});
