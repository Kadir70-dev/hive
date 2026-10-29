import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { readFileSync } from 'node:fs';
import { AdminMembers } from '@/components/admin/AdminMembers';

const data = {
  items: [
    { id: 'admin-1', first_name: 'Meera', email: 'meera@example.test', status: 'approved', is_admin: true, email_verified: true, created_at: '2026-10-01' },
    { id: 'member-1', first_name: 'Noor', email: 'noor@example.test', status: 'pending', is_admin: false, email_verified: true, created_at: '2026-10-02' },
  ],
  page: 1, page_size: 25, total: 2, has_more: false,
};

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminMembers /></QueryClientProvider>);
}

afterEach(() => vi.unstubAllGlobals());

describe('Admin workspace layout and controls', () => {
  it('keeps member search, API-backed status filtering and admin restrictions', async () => {
    const calls: { url: string; method: string; body?: unknown }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method || 'GET', body: init?.body ? JSON.parse(String(init.body)) : undefined });
      if (init?.method === 'PATCH') return new Response(JSON.stringify({ ok: true }), { status: 200 });
      return new Response(JSON.stringify(data), { status: 200 });
    }));
    mount();
    expect(await screen.findByText('Meera')).toBeInTheDocument();
    expect(screen.getByText('Noor')).toBeInTheDocument();
    expect(screen.getByRole('searchbox', { name: 'Search name or email' })).toBeInTheDocument();
    expect(screen.getByText('2 results')).toBeInTheDocument();
    expect(screen.getByText('Managed with the admin CLI')).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Status for meera@example.test' })).not.toBeInTheDocument();

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search name or email' }), { target: { value: 'Noor' } });
    await waitFor(() => expect(calls.some(c => c.url.includes('q=Noor'))).toBe(true));

    fireEvent.change(screen.getByRole('combobox', { name: 'Status' }), { target: { value: 'pending' } });
    await waitFor(() => expect(calls.some(c => c.url.includes('status=pending'))).toBe(true));

    fireEvent.change(screen.getByRole('combobox', { name: 'Status for noor@example.test' }), { target: { value: 'approved' } });
    await waitFor(() => expect(calls.some(c => c.method === 'PATCH'
      && c.url.includes('/admin/members/member-1')
      && JSON.stringify(c.body) === JSON.stringify({ status: 'approved' }))).toBe(true));
  });

  it('scopes larger admin layout styles to /admin, keeping the public member layout unchanged', () => {
    const css = readFileSync('src/styles.css', 'utf8');
    const screenSource = readFileSync('src/components/screens/AdminScreen.tsx', 'utf8');
    expect(css).toContain('.hv-wrap:has(> .hv-admin-page){max-width:1160px}');
    expect(css).toContain('.hv .hv-admin-search input[type="search"]');
    expect(screenSource).toContain('className="hv-admin-page"');
    expect(screenSource).toContain('className="hv-tabs hv-admin-tabs"');
    expect(screenSource).toContain('href="/studio"');
    expect(screenSource).toContain('<MfaGate me={me}><AdminTools /></MfaGate>');
  });
});
