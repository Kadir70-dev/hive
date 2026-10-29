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

describe('AuthScreen account flows', () => {
  it('shows "check your email" after sign-up instead of logging in', async () => {
    const { fireEvent, screen: s } = await import('@testing-library/react');
    renderAuth('mode=signup');
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"ok":true,"status":"verification_sent"}', { status: 202 })));
    fireEvent.change(s.getByLabelText(/First name/), { target: { value: 'Noor' } });
    fireEvent.change(s.getByLabelText(/Area in Abu Dhabi/), { target: { value: 'Al Reem Island' } });
    fireEvent.change(s.getByLabelText(/Email/), { target: { value: 'noor@example.test' } });
    fireEvent.change(s.getByLabelText(/Password/), { target: { value: 'a-long-password-123' } });
    fireEvent.click(s.getByRole('checkbox', { name: /I agree to the/ }));
    fireEvent.click(s.getByRole('button', { name: 'Create account' }));
    expect(await s.findByRole('heading', { name: 'Check your email' })).toBeInTheDocument();
  });

  it('offers a new confirmation link when the email is unverified', async () => {
    const { fireEvent, screen: s } = await import('@testing-library/react');
    renderAuth('');
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"please confirm your email address first","code":"email_unverified"}', { status: 403 })));
    fireEvent.change(s.getByLabelText(/Email/), { target: { value: 'noor@example.test' } });
    fireEvent.change(s.getByLabelText(/Password/), { target: { value: 'whatever' } });
    fireEvent.submit(s.getByLabelText(/Email/).closest('form')!);
    expect(await s.findByRole('button', { name: 'Send a new confirmation link' })).toBeInTheDocument();
    expect(s.getByRole('link', { name: 'Forgot your password?' })).toHaveAttribute('href', '/reset-password');
    expect(s.getByText('Please confirm your email address first')).toBeInTheDocument();
  });
});
