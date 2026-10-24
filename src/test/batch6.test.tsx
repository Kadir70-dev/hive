import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { ProfilePhoto } from '@/components/screens/ProfilePhoto';
import { AdminTools } from '@/components/screens/AdminScreen';
import { AdminSafetyConflicts } from '@/components/admin/AdminSafetyConflicts';
import type { Me } from '@/components/hive/HiveShell';

const me: Me = {
  id: 'user-1', first_name: 'Noor', email: 'noor@example.test',
  phone: null, area: '', interests: [], has_photo: false,
  photos_enabled: true, status: 'approved', isAdmin: false,
};
const wrap = (ui: ReactNode) => render(
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {ui}
  </QueryClientProvider>,
);
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe('private profile photo interactions', () => {
  it('disables controls when private storage is not configured', () => {
    wrap(<ProfilePhoto me={{ ...me, photos_enabled: false }} />);
    expect(screen.getByRole('status')).toHaveTextContent('not configured');
    expect(screen.queryByLabelText('Choose a photo')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /upload photo/i })).not.toBeInTheDocument();
  });

  it('rejects invalid file types without contacting the API', () => {
    const fetcher = vi.fn();
    vi.stubGlobal('fetch', fetcher);
    wrap(<ProfilePhoto me={me} />);
    const input = screen.getByLabelText('Choose a photo');
    fireEvent.change(input, { target: { files: [new File(['<svg/>'], 'bad.svg', { type: 'image/svg+xml' })] } });
    expect(screen.getByRole('alert')).toHaveTextContent('JPEG, PNG or WebP');
    expect(screen.getByRole('button', { name: 'Upload photo' })).toBeDisabled();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('uploads multipart data without overriding Content-Type', async () => {
    const fetcher = vi.fn(async () => Response.json({ ok: true }));
    vi.stubGlobal('fetch', fetcher);
    wrap(<ProfilePhoto me={me} />);
    const selected = new File(['fake jpeg bytes'], 'face.jpg', { type: 'image/jpeg' });
    fireEvent.change(screen.getByLabelText('Choose a photo'), { target: { files: [selected] } });
    fireEvent.click(screen.getByRole('button', { name: 'Upload photo' }));
    expect(await screen.findByText('Profile photo updated.')).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledTimes(1);
    const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/backend/me/photo');
    expect(init.method).toBe('POST');
    expect(init.headers).toBeUndefined();
    const file = (init.body as FormData).get('photo');
    expect(file).toBeInstanceOf(File);
    expect((file as File).name).toBe('face.jpg');
  });

  it('shows upload errors without pretending success', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'Photo storage is unavailable' }, { status: 503 })));
    wrap(<ProfilePhoto me={me} />);
    fireEvent.change(screen.getByLabelText('Choose a photo'), { target: { files: [new File(['x'], 'x.png', { type: 'image/png' })] } });
    fireEvent.click(screen.getByRole('button', { name: 'Upload photo' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Photo storage is unavailable');
    expect(screen.queryByText('Profile photo updated.')).not.toBeInTheDocument();
  });

  it('uses authenticated photo routes and removes a saved photo', async () => {
    const fetcher = vi.fn(async () => Response.json({ ok: true }));
    vi.stubGlobal('fetch', fetcher);
    wrap(<ProfilePhoto me={{ ...me, has_photo: true }} />);
    expect(screen.getByAltText('Your private profile photo')).toHaveAttribute('src', expect.stringContaining('/api/backend/me/photo'));
    fireEvent.click(screen.getByRole('button', { name: 'Remove photo' }));
    expect(await screen.findByText('Profile photo removed.')).toBeInTheDocument();
    const [, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    expect(init.method).toBe('DELETE');
  });
});

describe('admin safety-conflict actions', () => {
  it('requires explicit confirmation and sends an internal-only decision', async () => {
    const conflict = {
      gathering_id: 'g1', gathering_title: 'Majlis', starts_on: '2031-08-15', starts_at: '19:00:00',
      report_id: 'r1', reporter: { id: 'u1', first_name: 'Noor', status: 'confirmed' },
      reported: { id: 'u2', first_name: 'Amina', status: 'confirmed' },
    };
    let resolved = false;
    const sent: { method: string; body: unknown }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      if (url.endsWith('/admin/safety-conflicts') && method === 'GET') {
        return Response.json(resolved ? [] : [conflict]);
      }
      if (url.endsWith('/admin/safety-conflicts/resolve') && method === 'POST') {
        sent.push({ method, body: JSON.parse(String(init?.body)) });
        resolved = true;
        return Response.json({ ok: true });
      }
      return Response.json({ error: 'not found' }, { status: 404 });
    }));
    wrap(<AdminSafetyConflicts />);
    expect(await screen.findByText('Majlis')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Decision'), { target: { value: 'released_reported' } });
    fireEvent.change(screen.getByLabelText('Internal note (admin only)'), { target: { value: 'Separate future sessions' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save decision' }));
    const modal = screen.getByRole('alertdialog');
    expect(modal).toHaveTextContent('neutral email');
    expect(sent).toHaveLength(0);
    fireEvent.click(within(modal).getByRole('button', { name: 'Keep reviewing' }));
    expect(sent).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Save decision' }));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Release place' }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.body).toEqual({
      gatheringID: 'g1', reportID: 'r1', decision: 'released_reported', note: 'Separate future sessions',
    });
    expect(await screen.findByText('Safety decision recorded')).toBeInTheDocument();
  });
});

describe('admin keyboard accessibility', () => {
  it('supports roving tab focus with arrow keys, Home and End', () => {
    vi.stubGlobal('fetch', vi.fn(async () => Response.json({
      items: [], page: 1, page_size: 25, has_more: false, total: 0,
    })));
    wrap(<AdminTools />);
    const members = screen.getByRole('tab', { name: 'Members' });
    const gatherings = screen.getByRole('tab', { name: 'Gatherings' });
    const conflicts = screen.getByRole('tab', { name: 'Safety conflicts' });
    expect(members).toHaveAttribute('tabindex', '0');
    expect(gatherings).toHaveAttribute('tabindex', '-1');
    fireEvent.keyDown(members, { key: 'ArrowRight' });
    expect(gatherings).toHaveFocus();
    expect(gatherings).toHaveAttribute('aria-selected', 'true');
    fireEvent.keyDown(gatherings, { key: 'End' });
    expect(conflicts).toHaveFocus();
    fireEvent.keyDown(conflicts, { key: 'Home' });
    expect(members).toHaveFocus();
  });
});
