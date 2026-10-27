'use client';
import { useState, type FormEvent, type ReactNode } from 'react';
import Link from 'next/link';
import { send } from '@/lib/hive-api';

const message = (e: unknown, fallback: string) => (e instanceof Error ? e.message : fallback);

function Card({ title, children }: { title: string; children: ReactNode }) {
  return <main className="studio st-center"><div className="st-login"><h1>{title}</h1>{children}</div></main>;
}

/** Email + password sign-in; the API answers wrong details with one generic error. */
export function StudioLogin({ onDone }: { onDone: () => void }) {
  const [busy, setBusy] = useState(false), [error, setError] = useState('');
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, data = new FormData(form);
    setBusy(true); setError('');
    try {
      await send('/login', { email: String(data.get('email') || ''), password: String(data.get('password') || '') });
      onDone();
    } catch (err) { setError(message(err, 'Login failed')); }
    finally {
      const pw = form.elements.namedItem('password') as HTMLInputElement | null;
      if (pw) pw.value = '';
      setBusy(false);
    }
  }
  return <main className="studio st-center"><form className="st-login" onSubmit={submit}>
    <h1>Admin Login</h1>
    <input name="email" type="email" placeholder="Email" aria-label="Email" autoComplete="username" required />
    <input name="password" type="password" placeholder="Password" aria-label="Password" autoComplete="current-password" required />
    <button disabled={busy}>{busy ? 'Signing in…' : 'Login'}</button>
    {error && <p className="st-error" role="alert">{error}</p>}
  </form></main>;
}

/** Admin tools need a fresh authenticator (or recovery) code for this session. */
export function StudioMfa({ onDone, onLogout }: { onDone: () => void; onLogout: () => void }) {
  const [recovery, setRecovery] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState('');
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const value = String(new FormData(e.currentTarget).get('code') || '').trim();
    setBusy(true); setError('');
    try { await send('/mfa/verify', recovery ? { recoveryCode: value } : { code: value }); onDone(); }
    catch (err) { setError(message(err, 'Invalid code')); }
    finally { setBusy(false); }
  }
  return <main className="studio st-center"><form className="st-login" onSubmit={submit}>
    <h1>Two-factor check</h1>
    <input key={String(recovery)} name="code" aria-label={recovery ? 'Recovery code' : '6-digit code'} placeholder={recovery ? 'Recovery code' : '6-digit code'}
      inputMode={recovery ? 'text' : 'numeric'} autoComplete="one-time-code" required />
    <button disabled={busy}>{busy ? 'Checking…' : 'Continue'}</button>
    {error && <p className="st-error" role="alert">{error}</p>}
    <button type="button" className="st-text-btn" onClick={() => setRecovery(!recovery)}>{recovery ? 'Use an authenticator code' : 'Use a recovery code'}</button>
    <button type="button" className="st-text-btn" onClick={onLogout}>Logout</button>
  </form></main>;
}

export function StudioBlocked({ reason, onLogout }: { reason: 'not_admin' | 'unverified' | 'no_mfa'; onLogout: () => void }) {
  const text = {
    not_admin: 'This account cannot edit website content.',
    unverified: 'Confirm your email address before using admin tools.',
    no_mfa: 'Turn on two-factor authentication in the admin dashboard first, then come back here.',
  }[reason];
  return <Card title="Access restricted">
    <p className="st-muted">{text}</p>
    {reason === 'no_mfa' && <Link className="st-btn-accent" href="/admin">Open admin dashboard</Link>}
    <button type="button" onClick={onLogout}>Logout</button>
  </Card>;
}
