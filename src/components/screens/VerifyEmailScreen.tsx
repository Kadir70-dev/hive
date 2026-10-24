'use client';
import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { fragmentToken, send } from '@/lib/hive-api';
import { HiveShell } from '@/components/hive/HiveShell';

type State = 'working' | 'done' | 'failed';

export default function VerifyEmailScreen() {
  const [state, setState] = useState<State>('working');
  const [message, setMessage] = useState('');
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return; // tokens are single use; never submit twice
    started.current = true;
    const token = fragmentToken(window.location.hash);
    // Remove the token from the address bar and history.
    window.history.replaceState(null, '', window.location.pathname);
    const done = (s: State, m = '') => { setState(s); setMessage(m); };
    if (!token) { queueMicrotask(() => done('failed', 'This link is incomplete. Please use the link from your email.')); return; }
    send('/verify-email', { token })
      .then(() => done('done'))
      .catch((e: unknown) => done('failed', e instanceof Error ? e.message : 'Unable to confirm your email.'));
  }, []);
  return (
    <HiveShell>
      <div className="hv-hero">
        <h1>{state === 'done' ? 'Email confirmed' : state === 'failed' ? 'We could not confirm your email' : 'Confirming your email…'}</h1>
        {state === 'done' && <p>Thank you. You can now log in; new memberships are reviewed before approval.</p>}
        {state === 'failed' && <p role="alert">{message}</p>}
      </div>
      <div className="hv-card" style={{ maxWidth: 540, marginInline: 'auto' }}>
        <Link className="hv-btn" href="/auth">{state === 'failed' ? 'Log in to request a new link' : 'Log in'}</Link>
      </div>
    </HiveShell>
  );
}
