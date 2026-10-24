'use client';
import { useState, type FormEvent, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { send } from '@/lib/hive-api';
import type { Me } from '@/components/hive/HiveShell';

type Setup = { secret: string; otpauth_uri: string };

function Enrol({ onDone }: { onDone: () => void }) {
  const [setup, setSetup] = useState<Setup | null>(null), [codes, setCodes] = useState<string[]>([]);
  const [error, setError] = useState(''), [busy, setBusy] = useState(false);
  async function start() {
    setBusy(true); setError('');
    try { setSetup(await send<Setup>('/mfa/totp/setup', {})); } catch (e) { setError(e instanceof Error ? e.message : 'Setup failed'); } finally { setBusy(false); }
  }
  async function enable(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); setBusy(true); setError('');
    try { const r = await send<{ recovery_codes: string[] }>('/mfa/totp/enable', { code: String(new FormData(e.currentTarget).get('code') || '') }); setCodes(r.recovery_codes); }
    catch (err) { setError(err instanceof Error ? err.message : 'Invalid code'); } finally { setBusy(false); }
  }
  if (codes.length) return (
    <div className="hv-card"><h2>Save your recovery codes</h2>
      <p>Each code works once if you lose your phone. Store them somewhere safe; they will not be shown again.</p>
      <pre aria-label="Recovery codes">{codes.join('\n')}</pre>
      <button className="hv-btn" onClick={onDone}>I have saved them</button></div>);
  return (
    <div className="hv-card"><h2>Set up two-factor authentication</h2>
      <p>Admin tools need a code from an authenticator app (Google Authenticator, 1Password, Authy…).</p>
      {!setup ? <button className="hv-btn" disabled={busy} onClick={() => void start()}>Begin setup</button> : (
        <form className="hv-stack" onSubmit={enable}>
          <p>Add this key to your authenticator app, or <a href={setup.otpauth_uri}>open it in the app</a> on this device:</p>
          <code aria-label="Setup key" style={{ wordBreak: 'break-all' }}>{setup.secret.match(/.{1,4}/g)?.join(' ')}</code>
          <label>6-digit code from the app<input name="code" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" required /></label>
          <button className="hv-btn" disabled={busy}>Turn on two-factor</button>
        </form>)}
      {error && <p role="alert" className="hv-msg err">{error}</p>}
    </div>);
}

function Verify({ onDone }: { onDone: () => void }) {
  const [recovery, setRecovery] = useState(false), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); setBusy(true); setError('');
    const value = String(new FormData(e.currentTarget).get('code') || '');
    try { await send('/mfa/verify', recovery ? { recoveryCode: value } : { code: value }); onDone(); }
    catch (err) { setError(err instanceof Error ? err.message : 'Invalid code'); } finally { setBusy(false); }
  }
  return (
    <div className="hv-card"><h2>Two-factor check</h2>
      <form className="hv-stack" onSubmit={submit}>
        <label>{recovery ? 'Recovery code' : '6-digit code from your authenticator app'}
          <input key={String(recovery)} name="code" autoComplete="one-time-code" inputMode={recovery ? 'text' : 'numeric'} required /></label>
        {error && <p role="alert" className="hv-msg err">{error}</p>}
        <button className="hv-btn" disabled={busy}>Continue</button>
        <button type="button" className="hv-link" onClick={() => setRecovery(!recovery)}>{recovery ? 'Use an authenticator code' : 'Use a recovery code'}</button>
      </form></div>);
}

/** Shows enrolment or step-up verification until this session passes MFA. */
export function MfaGate({ me, children }: { me: Me; children: ReactNode }) {
  const qc = useQueryClient();
  const refresh = () => void qc.invalidateQueries({ queryKey: ['me'] });
  if (!me.email_verified) return <div className="hv-card">Confirm your email address before using admin tools.</div>;
  if (!me.mfa_enabled) return <Enrol onDone={refresh} />;
  if (!me.mfa_verified) return <Verify onDone={refresh} />;
  return <>{children}</>;
}
