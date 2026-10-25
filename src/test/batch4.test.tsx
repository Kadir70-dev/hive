import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useConfirm } from '@/components/hive/ui';
import GatheringsScreen from '@/components/screens/GatheringsScreen';
import SafetyScreen from '@/components/screens/SafetyScreen';
import ProfileScreen from '@/components/screens/ProfileScreen';
import CommunityListScreen from '@/components/screens/CommunityListScreen';
import { AdminPartners } from '@/components/admin/AdminPartners';
import { AdminAnnouncements } from '@/components/admin/AdminAnnouncements';

vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }), useSearchParams: () => new URLSearchParams() }));

type Call = { url: string; method: string; body: unknown };
let calls: Call[] = [];
function stub(routes: Record<string, (method: string, body: unknown) => [number, unknown]>) {
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ url, method, body });
    const path = url.replace('/api/backend', '').split('?')[0]!;
    const key = Object.keys(routes).find((k) => k === `${method} ${path}`) ?? Object.keys(routes).find((k) => k === path);
    const [status, payload] = key ? routes[key]!(method, body) : [404, { error: 'not found' }];
    return new Response(JSON.stringify(payload), { status });
  }));
}
const wrap = (ui: ReactNode) => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{ui}</QueryClientProvider>);
const member = { id: 'u1', first_name: 'Noor', email: 'n@x.test', phone: null, area: '', interests: [], avatar_path: null, status: 'approved', isAdmin: false, email_updates: false };
const gathering = (over: object) => ({ id: 'g1', title: 'Majlis', type: 'majlis', description: '', starts_on: '2031-01-01', starts_at: '19:00:00', place: 'Bateen', places_left: 3, my_status: '', waitlist_position: 0, started: false, ...over });

beforeEach(() => { calls = []; try { localStorage.removeItem('hive.site.lang'); } catch { /* ignore */ } });
afterEach(() => { vi.unstubAllGlobals(); });

function ConfirmHarness({ onResult }: { onResult: (ok: boolean) => void }) {
  const [confirm, dialog] = useConfirm();
  return <><button onClick={async () => onResult(await confirm({ title: 'Delete it?', body: 'Really?', confirmLabel: 'Delete', danger: true }))}>open</button>{dialog}</>;
}

describe('accessible confirm dialog', () => {
  it('is a labelled modal, focuses the safe action and closes on Escape', async () => {
    const onResult = vi.fn();
    render(<ConfirmHarness onResult={onResult} />);
    fireEvent.click(screen.getByText('open'));
    const dialog = screen.getByRole('alertdialog', { name: 'Delete it?' });
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleDescription('Really?');
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('resolves true on confirm', async () => {
    const onResult = vi.fn();
    render(<ConfirmHarness onResult={onResult} />);
    fireEvent.click(screen.getByText('open'));
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(onResult).toHaveBeenCalledWith(true));
  });
});

describe('gatherings screen', () => {
  it('shows waitlist position, books without alert() and cancels through a dialog', async () => {
    const alertSpy = vi.fn();
    vi.stubGlobal('alert', alertSpy);
    stub({
      '/me': () => [200, member],
      'GET /gatherings': () => [200, [gathering({}), gathering({ id: 'g2', title: 'Walk', places_left: 0, my_status: 'waitlist', waitlist_position: 2 }), gathering({ id: 'g3', title: 'Old', started: true })]],
      'POST /gatherings/g1/keep': () => [200, { ok: true, status: 'confirmed' }],
      'POST /gatherings/g2/cancel': () => [200, { ok: true }],
    });
    wrap(<GatheringsScreen />);
    expect(await screen.findByText('On the waitlist — position 2')).toBeInTheDocument();
    expect(screen.getByText('This gathering has started.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Keep my place' }));
    expect(await screen.findByRole('status')).toHaveTextContent("Your place is kept");
    fireEvent.click(screen.getByRole('button', { name: 'Leave the waitlist' }));
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/gatherings/g2/cancel'))).toBe(true));
    expect(alertSpy).not.toHaveBeenCalled();
  });

  it('requires women-only self-attestation before any new booking request', async () => {
    stub({
      '/me': () => [200, member],
      'GET /gatherings': () => [200, [gathering({ type: 'women-only' })]],
      'POST /gatherings/g1/keep': () => [200, { ok: true, status: 'confirmed' }],
    });
    wrap(<GatheringsScreen />);
    const confirmation = await screen.findByRole('checkbox', { name: /I confirm I meet this women-only gathering/ });
    const keep = screen.getByRole('button', { name: 'Keep my place' });
    expect(keep).toBeDisabled();
    expect(calls.some((c) => c.url.endsWith('/gatherings/g1/keep'))).toBe(false);
    fireEvent.click(confirmation);
    expect(keep).not.toBeDisabled();
    fireEvent.click(keep);
    await waitFor(() => expect(calls.find((c) => c.url.endsWith('/gatherings/g1/keep'))?.body)
      .toEqual({ womenOnlyAcknowledged: true }));
  });

  it('shows an error state with retry when loading fails', async () => {
    stub({ '/me': () => [200, member], 'GET /gatherings': () => [500, { error: 'boom' }] });
    wrap(<GatheringsScreen />);
    expect(await screen.findByRole('alert')).toHaveTextContent("We couldn't load this");
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });

  it('renders member labels in Arabic when chosen', async () => {
    localStorage.setItem('hive.site.lang', 'ar');
    stub({ '/me': () => [200, member], 'GET /gatherings': () => [200, [gathering({})]] });
    wrap(<GatheringsScreen />);
    expect(await screen.findByRole('button', { name: 'احجز مكاني' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'الأمان' })).toBeInTheDocument();
  });
});

describe('safety screen (admin-mediated, privacy-preserving)', () => {
  it('never asks for a member reference and submits a description instead', async () => {
    stub({
      '/me': () => [200, member],
      '/me/gatherings': () => [200, [{ id: 'g1', title: 'Majlis', starts_on: '2031-01-01' }]],
      'GET /reports/mine': () => [200, [{ id: 'r0', category: 'safety', gathering_title: '', status: 'in_review', created_at: '2031-01-02T00:00:00Z' }]],
      'POST /reports': () => [201, { ok: true, id: 'r1' }],
    });
    wrap(<SafetyScreen />);
    expect(await screen.findByText('Being reviewed')).toBeInTheDocument();
    expect(screen.queryByLabelText(/member reference/i)).not.toBeInTheDocument();
    await screen.findByRole('option', { name: /Majlis/ });
    fireEvent.change(screen.getByLabelText(/Which gathering/), { target: { value: 'g1' } });
    fireEvent.change(screen.getByLabelText(/Who was involved/), { target: { value: 'woman in the green scarf' } });
    fireEvent.change(screen.getByLabelText('What happened?'), { target: { value: 'Repeated unwanted comments.' } });
    fireEvent.click(screen.getByLabelText(/prefer not to be booked/));
    fireEvent.click(screen.getByRole('button', { name: 'Send confidential report' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({
      category: 'harassment', gatheringID: 'g1', subjectDescription: 'woman in the green scarf', reason: 'Repeated unwanted comments.', blockRequested: true,
    }));
    expect(await screen.findByText(/sent privately/)).toBeInTheDocument();
  });
});

describe('profile', () => {
  it('lists my gatherings with confirmed/waitlist status and explains email settings', async () => {
    stub({
      '/me': () => [200, member],
      '/me/gatherings': () => [200, [
        { id: 'g1', title: 'Majlis', starts_on: '2031-01-01', starts_at: '19:00:00', place: 'Bateen', status: 'confirmed', past: false, attended: false },
        { id: 'g2', title: 'Walk', starts_on: '2031-01-02', starts_at: '07:00:00', place: 'Corniche', status: 'waitlist', waitlist_position: 3, past: false, attended: false },
        { id: 'g3', title: 'Old', starts_on: '2020-01-02', starts_at: '07:00:00', place: 'X', status: 'confirmed', past: true, attended: true },
      ]],
      'PATCH /me': () => [200, { ok: true }],
    });
    wrap(<ProfileScreen />);
    expect(await screen.findByText('Your place is kept')).toBeInTheDocument();
    expect(screen.getByText('On the waitlist — position 3')).toBeInTheDocument();
    expect(screen.getByText('Attended')).toBeInTheDocument();
    const updates = screen.getByLabelText(/Email me announcements/);
    expect(updates).toHaveAccessibleDescription(/Announcements are emailed only when this is on/);
    fireEvent.click(updates);
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect((calls.find((c) => c.method === 'PATCH')?.body as { emailUpdates: boolean }).emailUpdates).toBe(true));
  });
});

describe('announcement feed', () => {
  it('pages with "Show more"', async () => {
    stub({ '/me': () => [200, member], 'GET /announcements': () => {
      const page = calls.filter((c) => c.url.includes('/announcements')).length;
      return [200, { items: [{ id: `a${page}`, title: `News ${page}`, body: 'b', published_at: '2031-01-01T00:00:00Z' }], page, has_more: page < 2 }];
    } });
    wrap(<CommunityListScreen kind="announcements" />);
    expect(await screen.findByText('News 1')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));
    expect(await screen.findByText('News 2')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument());
  });
});

describe('admin dashboard', () => {
  it('rejects non-https partner links before calling the API', async () => {
    stub({ 'GET /admin/partners': () => [200, { items: [], page: 1, page_size: 25, total: 0, has_more: false }] });
    wrap(<AdminPartners />);
    fireEvent.click(await screen.findByText('Add a partner'));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Cafe' } });
    fireEvent.change(screen.getByLabelText('Website (https://)'), { target: { value: 'javascript:alert(1)' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add partner' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('https://');
    expect(calls.some((c) => c.method === 'POST')).toBe(false);
  });

  it('asks before emailing an announcement and reports the queued count', async () => {
    stub({
      'GET /admin/announcements': () => [200, { items: [{ id: 'a1', title: 'Season', body: 'b', published: true, created_at: '', published_at: '', emailed_count: 0 }], page: 1, page_size: 25, total: 1, has_more: false }],
      'POST /admin/announcements/a1/email': () => [200, { ok: true, queued: 4 }],
    });
    wrap(<AdminAnnouncements />);
    fireEvent.click(await screen.findByRole('button', { name: 'Email opted-in members' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Email this announcement?' });
    expect(dialog).toHaveTextContent('turned on email updates');
    await act(async () => { fireEvent.click(within(dialog).getByRole('button', { name: 'Send emails' })); });
    expect(await screen.findByText('4 emails queued')).toBeInTheDocument();
  });
});
