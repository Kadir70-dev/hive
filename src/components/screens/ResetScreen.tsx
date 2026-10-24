'use client';
import { useState, useSyncExternalStore, type FormEvent } from 'react';
import Link from 'next/link';
import { fragmentToken, send } from '@/lib/hive-api';
import { HiveShell } from '@/components/hive/HiveShell';

// The token arrives in the URL fragment; read it once on the client.
let capturedToken: string | null = null;
function readToken(): string {
  if (capturedToken === null) {
    capturedToken = fragmentToken(window.location.hash);
    if (capturedToken) window.history.replaceState(null, '', window.location.pathname);
  }
  return capturedToken;
}
const noop = () => () => {};

function RequestLink() {
  const [sent, setSent] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState('');
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); setBusy(true); setError('');
    try { await send('/password/forgot', { email: String(new FormData(e.currentTarget).get('email') || '') }); setSent(true); }
    catch (err) { setError(err instanceof Error ? err.message : 'Unable to send'); }
    finally { setBusy(false); }
  }
  if (sent) return <p role="status">If an account uses that address, a reset link is on its way. It expires in 30 minutes.</p>;
  return <form className="hv-stack" onSubmit={submit}><label>Email<input name="email" type="email" required autoComplete="email" /></label>{error && <p role="alert" className="hv-msg err">{error}</p>}<button className="hv-btn" disabled={busy}>{busy ? 'Please wait…' : 'Email me a reset link'}</button></form>;
}

function ChooseNew({ token, onRestart }: { token: string; onRestart: () => void }) {
  const [done, setDone] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState('');
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); setError('');
    const f = new FormData(e.currentTarget), password = String(f.get('password') || '');
    if (password !== String(f.get('confirm') || '')) { setError('The two passwords do not match.'); return; }
    setBusy(true);
    try { await send('/password/reset', { token, password }); setDone(true); }
    catch (err) { setError(err instanceof Error ? err.message : 'Unable to reset'); }
    finally { setBusy(false); }
  }
  if (done) return <div className="hv-stack"><p role="status">Your password was changed and every device was signed out.</p><Link className="hv-btn" href="/auth">Log in</Link></div>;
  return <form className="hv-stack" onSubmit={submit}><label>New password (12+ characters)<input name="password" type="password" required minLength={12} maxLength={128} autoComplete="new-password" /></label><label>Repeat new password<input name="confirm" type="password" required minLength={12} maxLength={128} autoComplete="new-password" /></label>{error && <p role="alert" className="hv-msg err">{error}</p>}<button className="hv-btn" disabled={busy}>{busy ? 'Please wait…' : 'Set new password'}</button>{error && <button type="button" className="hv-link" onClick={onRestart}>Request a new link</button>}</form>;
}

export default function ResetScreen() {
  const captured = useSyncExternalStore(noop, readToken, () => '');
  const [restarted, setRestarted] = useState(false);
  const token = restarted ? '' : captured;
  return <HiveShell><div className="hv-hero"><h1>{token ? 'Choose a new password' : 'Reset your password'}</h1></div><div className="hv-card" style={{ maxWidth: 540, marginInline: 'auto' }}>{token ? <ChooseNew token={token} onRestart={() => setRestarted(true)} /> : <RequestLink />}</div></HiveShell>;
}
