import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { readFileSync } from 'node:fs';
import { partnerInitials, approvedPartnerSite } from '@/components/screens/CommunityListScreen';
import AuthScreen from '@/components/screens/AuthScreen';
import Terms from '@/app/terms/page';
import PrivacyPage from '@/components/screens/PrivacyScreen';
import { arabicTerms, arabicPrivacy } from '@/lib/hive-policy-ar';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  useSearchParams: () => new URLSearchParams('mode=signup'),
}));
afterEach(() => { vi.unstubAllGlobals(); localStorage.removeItem('hive.site.lang'); });

describe('Meera frontend requirements', () => {
  it('updates PWA branding for everyone, not only women', () => {
    const manifest = JSON.parse(readFileSync('public/manifest.webmanifest', 'utf8')) as {description: string; name: string; icons: {src:string}[]};
    expect(manifest.name).toBe('The Hive Society');
    expect(manifest.description).toContain('men and women');
    expect(manifest.icons.map((i) => i.src)).toContain('/icon-192.png?v=2');
    expect(manifest.icons.map((i) => i.src)).toContain('/icon-maskable-512.png?v=2');
  });

  it('renders 18+ inclusive legal text and an Arabic policy draft', () => {
    const privacy = renderToStaticMarkup(<PrivacyPage />);
    const terms = renderToStaticMarkup(<Terms />);
    expect(privacy).toContain('Membership welcomes adults aged 18 and over');
    expect(terms).toContain('all nationalities');
    expect(arabicTerms[1]!.body.join(' ')).toContain('نساءً');
    expect(arabicPrivacy.at(-2)!.body.join(' ')).toContain('رجالاً');
    expect(privacy).not.toContain('for women aged 18 and over');
  });

  it('uses informative partner monograms, rejects unsafe URLs', () => {
    expect(partnerInitials('Abu Dhabi Museum')).toBe('AD');
    expect(partnerInitials('')).toBe('H');
    expect(approvedPartnerSite('javascript:alert(1)')).toBeNull();
    expect(approvedPartnerSite('http://example.com')).toBeNull();
    expect(approvedPartnerSite('https://example.com/')).toBe('https://example.com/');
  });

  it('allows optional interests and sends chosen values in registration', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const calls: unknown[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/signup') && init?.body) calls.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify(url.endsWith('/signup') ? {ok:true,status:'verification_sent'} : {}), { status: url.endsWith('/signup') ? 202 : 401 });
    }));
    render(<QueryClientProvider client={qc}><AuthScreen /></QueryClientProvider>);
    fireEvent.change(screen.getByLabelText(/First name/), {target: {value: 'Noor'}});
    fireEvent.change(screen.getByLabelText(/Email/), {target: {value: 'noor@example.test'}});
    fireEvent.change(screen.getByLabelText(/Password/), {target: {value: 'a-secure-password-123'}});
    fireEvent.click(screen.getByLabelText('Culture & heritage'));
    fireEvent.click(screen.getByRole('checkbox', {name: /I agree to the/}));
    fireEvent.click(screen.getByRole('button', {name: 'Create account'}));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]).toEqual(expect.objectContaining({interests: ['culture']}));
  });
});
