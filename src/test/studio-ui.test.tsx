import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import StudioApp from '@/components/studio/StudioApp';

const template = readFileSync(path.join(process.cwd(), 'public/hive.html'), 'utf8');
const admin = { isAdmin: true, email_verified: true, mfa_enabled: true, mfa_verified: true };
const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { 'content-type': 'application/json' } });
type Call = { url: string; method: string; body?: unknown };

function server(opts: { me?: unknown; meStatus?: number } = {}) {
  const calls: Call[] = [];
  let draftRev = 0;
  const empty = { en: {}, ar: {}, seo: {} };
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input), method = init.method || 'GET';
    calls.push({ url, method, body: typeof init.body === 'string' ? JSON.parse(init.body) : undefined });
    if (url === '/hive.html') return new Response(template, { status: 200 });
    if (url.endsWith('/me')) return opts.meStatus ? json({ error: 'not signed in' }, opts.meStatus) : json(opts.me ?? admin);
    if (url.endsWith('/login')) return json({ error: 'invalid email or password', code: 'invalid_credentials' }, 401);
    if (url.endsWith('/admin/cms/pages/home')) return json({ draft: empty, published: empty, draft_revision: draftRev, published_revision: 0, published_at: null });
    if (url.endsWith('/versions')) return json({ data: [] });
    if (url.endsWith('/draft')) { draftRev += 1; return json({ ok: true, draft_revision: draftRev }); }
    return json({ error: 'not found' }, 404);
  }));
  return calls;
}

function renderStudio() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><StudioApp /></QueryClientProvider>);
}

afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState(null, '', '/'); });

describe('Website Studio', () => {
  it('asks for email and password only, and shows a generic error for wrong details', async () => {
    server({ meStatus: 401 });
    renderStudio();
    expect(await screen.findByRole('heading', { name: 'Admin Login' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'qa-studio@example.test' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'wrong-password-123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid email or password');
    expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe('');
  });

  it('keeps non-admins out', async () => {
    server({ me: { ...admin, isAdmin: false } });
    renderStudio();
    expect(await screen.findByText('This account cannot edit website content.')).toBeInTheDocument();
  });

  it('opens with the live website copy and saves only the edited text as a draft', async () => {
    const calls = server();
    renderStudio();
    const title = await screen.findByRole('button', { name: 'Edit Menu link: why we exist' });
    expect(title).toHaveTextContent('Why we exist');
    fireEvent.click(screen.getByRole('button', { name: 'Hero' }));
    const hero = await screen.findByRole('button', { name: 'Edit Hero title' });
    expect(hero).toHaveTextContent('The art of getting together');
    expect(hero.querySelector('em')).toHaveTextContent('getting together');

    // Esc cancels an edit.
    fireEvent.click(screen.getByRole('button', { name: 'Edit Tagline' }));
    fireEvent.change(screen.getByLabelText('Tagline'), { target: { value: 'QA-TEST discarded' } });
    fireEvent.keyDown(screen.getByLabelText('Tagline'), { key: 'Escape' });
    expect(screen.getByRole('button', { name: 'Edit Tagline' })).toHaveTextContent('One hand, for our Abu Dhabi.');
    expect(screen.getByText('All changes saved', { exact: false })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Edit Tagline' }));
    fireEvent.change(screen.getByLabelText('Tagline'), { target: { value: 'QA-TEST tagline' } });
    fireEvent.keyDown(screen.getByLabelText('Tagline'), { key: 'Enter' });
    expect(screen.getByText('● Unsaved changes')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('The live website is unchanged until you publish.'));
    const put = calls.find((c) => c.method === 'PUT');
    expect(put?.url).toBe('/api/backend/admin/cms/pages/home/draft');
    expect(put?.body).toMatchObject({ expected_revision: 0, document: { en: { heroLine: 'QA-TEST tagline' }, ar: {} } });
    expect(calls.some((c) => c.url.endsWith('/publish'))).toBe(false);
  });

  it('closes a multi-line editor with Enter instead of adding a line break', async () => {
    server();
    renderStudio();
    await screen.findByRole('button', { name: 'Edit Menu link: why we exist' });
    fireEvent.click(screen.getByRole('button', { name: 'Hero' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Introduction' }));
    const box = screen.getByLabelText('Introduction');
    expect(box.tagName).toBe('TEXTAREA');
    fireEvent.change(box, { target: { value: 'QA-TEST\nintro' } });
    fireEvent.keyDown(box, { key: 'Enter' });
    expect(screen.queryByLabelText('Introduction')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit Introduction' })).toHaveTextContent('QA-TEST intro');
  });

  it('switches to Arabic copy right to left', async () => {
    server();
    renderStudio();
    await screen.findByRole('button', { name: 'Edit Menu link: why we exist' });
    fireEvent.click(screen.getByRole('button', { name: 'العربية' }));
    const canvas = await screen.findByLabelText('Header & Menu preview');
    expect(canvas).toHaveAttribute('dir', 'rtl');
    expect(screen.getByRole('button', { name: 'Edit Menu link: why we exist' })).toHaveTextContent('لماذا وُجدنا');
  });
});
