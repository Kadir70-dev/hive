'use client';

import { useEffect, useState, type ChangeEvent } from 'react';
import Image from 'next/image';
import { useQueryClient } from '@tanstack/react-query';
import type { Me } from '@/components/hive/HiveShell';
import { Notice } from '@/components/hive/ui';

const allowed = new Set(['image/jpeg', 'image/png', 'image/webp']);
const maximum = 5 * 1024 * 1024;

/** Photos are always fetched through the session-authenticated Go API. */
export function ProfilePhoto({ me }: { me: Me }) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<File | null>(null);
  const [preview, setPreview] = useState('');
  const [version, setVersion] = useState(0);
  const [photoError, setPhotoError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ text: string; tone: 'ok' | 'err' }>({ text: '', tone: 'ok' });

  useEffect(() => () => { if (preview) URL.revokeObjectURL(preview); }, [preview]);

  function choose(e: ChangeEvent<HTMLInputElement>) {
    const chosen = e.currentTarget.files?.[0];
    e.currentTarget.value = '';
    if (!chosen) return;
    setSelected(null);
    setPreview('');
    setMessage({ text: '', tone: 'ok' });
    setPhotoError(false);
    if (!allowed.has(chosen.type) || chosen.size > maximum || chosen.size === 0) {
      setMessage({ text: 'Choose a JPEG, PNG or WebP photo up to 5 MB.', tone: 'err' });
      return;
    }
    setSelected(chosen);
    if (typeof URL.createObjectURL === 'function') setPreview(URL.createObjectURL(chosen));
  }

  async function requestPhoto(method: 'POST' | 'DELETE') {
    if (method === 'POST' && !selected) return;
    setBusy(true);
    setMessage({ text: '', tone: 'ok' });
    try {
      const payload = new FormData();
      if (method === 'POST' && selected) payload.append('photo', selected, selected.name);
      // Do not set Content-Type: fetch must supply the multipart boundary.
      const response = await fetch('/api/backend/me/photo', {
        method, credentials: 'same-origin', cache: 'no-store',
        ...(method === 'POST' ? { body: payload } : {}),
      });
      if (!response.ok) {
        const detail = await response.json().catch(() => ({})) as { error?: string };
        throw new Error(detail.error || 'Could not update your profile photo');
      }
      setSelected(null);
      setPreview('');
      setVersion((v) => v + 1);
      setPhotoError(false);
      await qc.invalidateQueries({ queryKey: ['me'] });
      setMessage({ text: method === 'POST' ? 'Profile photo updated.' : 'Profile photo removed.', tone: 'ok' });
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Photo update failed. Try again.', tone: 'err' });
    } finally {
      setBusy(false);
    }
  }

  if (!me.photos_enabled) {
    return (
      <section className="hv-card" aria-label="Your profile photo">
        <h2>Your profile photo</h2>
        <p className="hv-field-hint" role="status">Photo uploads are not configured yet. Your other profile details work normally.</p>
      </section>
    );
  }

  const src = photoError ? '' : (preview || (me.has_photo ? `/api/backend/me/photo?v=${version}` : ''));
  return (
    <section className="hv-card" aria-labelledby="profile-photo-heading" aria-busy={busy}>
      <h2 id="profile-photo-heading">Your profile photo</h2>
      <div className="hv-photo-layout">
        {src
          ? <Image className="hv-avatar" src={src} width={112} height={112} unoptimized alt={preview ? 'Selected photo preview' : 'Your private profile photo'} onError={() => setPhotoError(true)} />
          : <div className="hv-avatar" aria-hidden="true">{me.first_name.slice(0, 1).toUpperCase()}</div>}
        <div className="hv-stack">
          <label className="hv-photo-label">Choose a photo
            <input type="file" accept="image/jpeg,image/png,image/webp" onChange={choose} disabled={busy} aria-describedby="photo-privacy" />
          </label>
          <p id="photo-privacy" className="hv-field-hint">JPEG, PNG or WebP, 5 MB maximum. We remove location metadata. Only you and authorized administrators can view your saved photo.</p>
          {selected && <p className="hv-muted">Selected: {selected.name}</p>}
          <div className="hv-actions">
            <button className="hv-btn sm" type="button" onClick={() => void requestPhoto('POST')} disabled={busy || !selected}>
              {busy ? 'Working…' : me.has_photo ? 'Replace photo' : 'Upload photo'}
            </button>
            {me.has_photo && <button className="hv-btn danger sm" type="button" onClick={() => void requestPhoto('DELETE')} disabled={busy}>Remove photo</button>}
          </div>
          <Notice message={message.text} tone={message.tone} />
          {photoError && <p className="hv-msg err" role="alert">The photo could not be displayed. The private storage service may be temporarily unavailable.</p>}
        </div>
      </div>
    </section>
  );
}
