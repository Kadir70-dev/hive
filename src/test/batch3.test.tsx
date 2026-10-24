import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { renderToStaticMarkup } from 'react-dom/server';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextRequest } from 'next/server';
import { GatheringEditor } from '@/components/screens/GatheringEditor';
import PrivacyPage from '@/components/screens/PrivacyScreen';
import Terms from '@/app/terms/page';
import ProfileScreen from '@/components/screens/ProfileScreen';
import { GET as proxyGET } from '@/app/api/backend/[...path]/route';

vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }), useSearchParams: () => new URLSearchParams() }));
afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

describe('GatheringEditor', () => {
  it('submits every field with a numeric capacity and closes on success', async () => {
    const onSave = vi.fn(async () => true), onClose = vi.fn();
    render(<GatheringEditor g={{ id: 'g1', title: 'Walk', type: 'wander', description: '', starts_on: '2031-04-01', starts_at: '07:00:00', place: 'Corniche', capacity: 4 }} onSave={onSave} onClose={onClose} />);
    expect(screen.getByLabelText('Time')).toHaveValue('07:00');
    fireEvent.change(screen.getByLabelText('Capacity'), { target: { value: '6' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ title: 'Walk', startsOn: '2031-04-01', startsAt: '07:00', place: 'Corniche', capacity: 6 }));
  });

  it('stays open when the API refuses the change', async () => {
    const onClose = vi.fn();
    render(<GatheringEditor g={{ id: 'g1', title: 'Walk', type: '', description: '', starts_on: '2031-04-01', starts_at: '07:00', place: 'C', capacity: 4 }} onSave={async () => false} onClose={onClose} />);
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save changes' })).not.toBeDisabled());
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe('legal drafts', () => {
  const privacy = renderToStaticMarkup(<PrivacyPage />);
  const terms = renderToStaticMarkup(<Terms />);
  it('are clearly marked as drafts and mention no retired vendors', () => {
    for (const html of [privacy, terms]) {
      expect(html).toMatch(/Draft for review/);
      expect(html.toLowerCase()).not.toMatch(/lovable|sb-.*auth-token/);
    }
  });
  it('privacy draft describes the real processors, cookie and retention', () => {
    for (const s of ['Hetzner', 'Netlify', 'Resend', 'Google Fonts', 'hive_session', '14 days', 'Argon2id', 'two-factor']) expect(privacy).toContain(s);
    expect(privacy).not.toMatch(/fully compliant|complies with/i);
  });
});

describe('account deletion', () => {
  it('requires the current password before deleting', async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (url.endsWith('/me') && init?.method !== 'DELETE') return new Response(JSON.stringify({ id: 'u1', first_name: 'N', email: 'n@x.test', phone: null, area: '', interests: [], avatar_path: null, status: 'approved', isAdmin: false }), { status: 200 });
      return new Response('{"ok":true}', { status: 200 });
    }));
    render(<QueryClientProvider client={new QueryClient()}><ProfileScreen /></QueryClientProvider>);
    fireEvent.click(await screen.findByRole('button', { name: 'Delete my account' }));
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'my-current-password' } });
    fireEvent.click(screen.getByRole('button', { name: 'Delete permanently' }));
    await waitFor(() => expect(calls.some(c => c.init?.method === 'DELETE')).toBe(true));
    const del = calls.find(c => c.init?.method === 'DELETE')!;
    expect(JSON.parse(String(del.init!.body))).toEqual({ password: 'my-current-password' });
  });
});

describe('backend proxy route', () => {
  it('refuses dot segments so only /v1 routes are reachable', async () => {
    vi.stubEnv('HIVE_API_INTERNAL_URL', 'https://api.example.test');
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    for (const path of [['..', 'health'], ['.', 'me'], ['']]) {
      const res = await proxyGET(new NextRequest('https://x.test/api/backend/x'), { params: Promise.resolve({ path }) });
      expect(res.status).toBe(404);
    }
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
