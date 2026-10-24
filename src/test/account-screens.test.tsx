import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import VerifyEmailScreen from '@/components/screens/VerifyEmailScreen';
import { MfaGate } from '@/components/screens/MfaGate';
import type { Me } from '@/components/hive/HiveShell';

vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

const calls: { url: string; body: unknown }[] = [];
function stubApi(handler: (url: string) => [number, unknown]) {
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const [status, body] = handler(url);
    return new Response(JSON.stringify(body), { status });
  }));
}
const wrap = (ui: ReactNode) => render(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);

beforeEach(() => { calls.length = 0; });
afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState(null, '', '/'); });

describe('VerifyEmailScreen', () => {
  it('submits the fragment token once and removes it from the URL', async () => {
    window.history.replaceState(null, '', '/verify-email#token=tok_abcdefghijklmnopqrstuvwxyz');
    stubApi(url => (url.endsWith('/verify-email') ? [200, { ok: true }] : [401, {}]));
    wrap(<VerifyEmailScreen />);
    expect(await screen.findByRole('heading', { name: 'Email confirmed' })).toBeInTheDocument();
    const verifies = calls.filter(c => c.url.endsWith('/verify-email'));
    expect(verifies).toHaveLength(1);
    expect(verifies[0]!.body).toEqual({ token: 'tok_abcdefghijklmnopqrstuvwxyz' });
    expect(window.location.hash).toBe('');
  });

  it('explains an invalid link', async () => {
    window.history.replaceState(null, '', '/verify-email#token=tok_expiredexpiredexpiredexp');
    stubApi(url => (url.endsWith('/verify-email') ? [400, { error: 'this link is invalid or has expired; request a new one', code: 'invalid_token' }] : [401, {}]));
    wrap(<VerifyEmailScreen />);
    expect(await screen.findByRole('alert')).toHaveTextContent('invalid or has expired');
  });
});

const admin = (over: Partial<Me>): Me => ({ id: '1', first_name: 'A', email: 'a@x.test', phone: null, area: '', interests: [], has_photo: false, photos_enabled: false, status: 'approved', isAdmin: true, email_verified: true, mfa_enabled: true, mfa_verified: true, ...over });

describe('MfaGate', () => {
  it('shows admin tools only after MFA verification', () => {
    wrap(<MfaGate me={admin({})}><p>admin tools</p></MfaGate>);
    expect(screen.getByText('admin tools')).toBeInTheDocument();
  });

  it('asks for a code when this session is not verified', async () => {
    stubApi(() => [200, { ok: true }]);
    wrap(<MfaGate me={admin({ mfa_verified: false })}><p>admin tools</p></MfaGate>);
    expect(screen.queryByText('admin tools')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/6-digit code/), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    await waitFor(() => expect(calls.find(c => c.url.endsWith('/mfa/verify'))?.body).toEqual({ code: '123456' }));
  });

  it('walks through enrolment and shows recovery codes once', async () => {
    stubApi(url => url.endsWith('/setup') ? [200, { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/x' }] : [200, { recovery_codes: ['aaaa-bbbb-cccc'] }]);
    wrap(<MfaGate me={admin({ mfa_enabled: false, mfa_verified: false })}><p>admin tools</p></MfaGate>);
    fireEvent.click(screen.getByRole('button', { name: 'Begin setup' }));
    expect(await screen.findByLabelText('Setup key')).toHaveTextContent('JBSW Y3DP EHPK 3PXP');
    fireEvent.change(screen.getByLabelText(/6-digit code/), { target: { value: '654321' } });
    fireEvent.click(screen.getByRole('button', { name: 'Turn on two-factor' }));
    expect(await screen.findByLabelText('Recovery codes')).toHaveTextContent('aaaa-bbbb-cccc');
  });

  it('blocks unverified admin accounts', () => {
    wrap(<MfaGate me={admin({ email_verified: false })}><p>admin tools</p></MfaGate>);
    expect(screen.getByText(/Confirm your email/)).toBeInTheDocument();
  });
});
