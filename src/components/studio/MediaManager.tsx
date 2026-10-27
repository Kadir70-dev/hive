'use client';
import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, send } from '@/lib/hive-api';
import { safeImageURL } from '@/lib/cms-home';

export type Media = { id: string; url: string; filename: string; alt_text: string; bytes: number; width: number; height: number };
type Crop = 'original' | 'wide' | 'square';
type Props = { onPick?: (m: { url: string; alt: string }) => void; onClose?: () => void };

const TYPES = /^image\/(png|jpeg|webp)$/;
const MAX = 5 << 20;

async function cropToWebP(file: File, crop: Crop): Promise<File> {
  if (crop === 'original') return file;
  const bitmap = await createImageBitmap(file);
  try {
    const ratio = crop === 'wide' ? 16 / 9 : 1;
    const w = Math.min(bitmap.width, bitmap.height * ratio), h = w / ratio;
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(Math.min(w, 2400)));
    canvas.height = Math.max(1, Math.round(canvas.width / ratio));
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('Image editing is not available in this browser');
    ctx.drawImage(bitmap, (bitmap.width - w) / 2, (bitmap.height - h) / 2, w, h, 0, 0, canvas.width, canvas.height);
    const blob = await new Promise<Blob>((ok, fail) => canvas.toBlob((b) => (b ? ok(b) : fail(new Error('Image conversion failed'))), 'image/webp', 0.86));
    return new File([blob], file.name.replace(/\.[^.]+$/, '') + '-cropped.webp', { type: 'image/webp' });
  } finally { bitmap.close(); }
}

async function uploadOne(file: File): Promise<Media> {
  const form = new FormData();
  form.append('file', file);
  const r = await fetch('/api/backend/admin/cms/media', { method: 'POST', body: form, credentials: 'same-origin', cache: 'no-store' });
  const body = await r.json().catch(() => ({}));
  if (!r.ok) throw new ApiError(body.error || 'Upload failed', r.status, body.code);
  return body as Media;
}

/** Image library: the full manager in the Studio, or an image picker when onPick is set. */
export default function MediaManager({ onPick, onClose }: Props) {
  const qc = useQueryClient();
  const library = useQuery({ queryKey: ['cms-media'], queryFn: () => api<{ data: Media[] }>('/admin/cms/media') });
  const media = library.data?.data ?? [], loading = library.isPending;
  const load = () => qc.invalidateQueries({ queryKey: ['cms-media'] });
  const [busy, setBusy] = useState(false), [progress, setProgress] = useState(''), [error, setError] = useState(''), [note, setNote] = useState('');
  const [crop, setCrop] = useState<Crop>('original'), [alts, setAlts] = useState<Record<string, string>>({});
  const [url, setUrl] = useState(''), [urlAlt, setUrlAlt] = useState(''), [drag, setDrag] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const picker = !!onPick;


  async function upload(files: FileList | File[]) {
    const list = Array.from(files);
    const bad = list.find((f) => !TYPES.test(f.type) || f.size > MAX);
    if (!list.length || bad) { setError('Choose JPEG, PNG or WebP images of 5 MB or less.'); return; }
    setBusy(true); setError(''); setNote('');
    try {
      for (const [i, file] of list.entries()) {
        setProgress(`Uploading ${i + 1} of ${list.length}…`);
        await uploadOne(await cropToWebP(file, crop));
      }
      await load();
      setNote(`${list.length} image${list.length > 1 ? 's' : ''} uploaded. Location data and other metadata were removed.`);
    } catch (e) { setError(e instanceof Error ? e.message : 'Upload failed'); }
    finally { setBusy(false); setProgress(''); }
  }

  async function saveAlt(m: Media) {
    setBusy(true); setError('');
    try { await send(`/admin/cms/media/${m.id}`, { alt_text: alts[m.id] ?? m.alt_text }, 'PATCH'); await load(); setNote('Image description saved.'); }
    catch (e) { setError(e instanceof Error ? e.message : 'Could not save the description'); }
    finally { setBusy(false); }
  }

  async function copy(m: Media) {
    try { await navigator.clipboard.writeText(new URL(m.url, location.origin).href); setNote('Image URL copied.'); }
    catch { setError('Copying is blocked in this browser; select the address instead.'); }
  }

  return <div className={picker ? 'st-media st-media-picker' : 'st-media'}>
    <div className="st-media-head">
      <div><h2>{picker ? 'Choose an image' : 'Image Uploads & Url'}</h2>
        <p>{picker ? 'Pick from the library, upload a new image or paste an image address.' : 'Upload and describe the images used on the website.'}</p></div>
      {onClose && <button type="button" className="st-close" onClick={onClose} aria-label="Close">✕</button>}
    </div>
    <div className={`st-drop${drag ? ' st-drag' : ''}`} onDragOver={(e) => { e.preventDefault(); setDrag(true); }} onDragLeave={() => setDrag(false)}
      onDrop={(e) => { e.preventDefault(); setDrag(false); void upload(e.dataTransfer.files); }}>
      <p className="st-drop-title">{progress || 'Drag images here or'}</p>
      <button type="button" className="st-grad" disabled={busy} onClick={() => input.current?.click()}>{busy ? 'Uploading…' : 'Select images'}</button>
      <input ref={input} hidden type="file" multiple accept="image/png,image/jpeg,image/webp" onChange={(e) => { if (e.target.files) void upload(e.target.files); e.target.value = ''; }} />
      <label className="st-crop">Upload mode
        <select value={crop} onChange={(e) => setCrop(e.target.value as Crop)}>
          <option value="original">Original image</option>
          <option value="wide">Centre crop 16:9 (WebP)</option>
          <option value="square">Centre crop 1:1 (WebP)</option>
        </select>
      </label>
      <small>JPG, PNG or WebP · 5 MB max · stored at up to 2400px</small>
    </div>
    {picker && <form className="st-url" onSubmit={(e) => {
      e.preventDefault();
      const v = url.trim();
      if (!v || !safeImageURL(v)) { setError('Paste an https:// image address (or a /path on this site).'); return; }
      onPick!({ url: v, alt: urlAlt.trim() });
    }}>
      <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="…or paste an https:// image URL" aria-label="Image URL" />
      <input value={urlAlt} onChange={(e) => setUrlAlt(e.target.value)} placeholder="Image description (alt text)" aria-label="Image description" maxLength={300} />
      <button type="submit">Use URL</button>
    </form>}
    {(error || library.error) && <p className="st-error" role="alert">{error || library.error?.message}</p>}
    {note && <p className="st-note" role="status">{note}</p>}
    {loading ? <p className="st-muted">Loading library…</p> : !media.length ? <p className="st-muted">No images uploaded yet.</p> :
      <div className="st-media-grid">{media.map((m) => <div key={m.id} className="st-media-card">
        {/* eslint-disable-next-line @next/next/no-img-element -- admin-chosen URLs (library or https) are shown as-is */}
        <div className="st-media-img"><img src={m.url} alt={m.alt_text || m.filename} loading="lazy" />
          {picker && <button type="button" className="st-use" onClick={() => onPick!({ url: m.url, alt: m.alt_text })}>Use this image</button>}
        </div>
        <div className="st-media-body">
          <p title={m.filename}>{m.filename}</p>
          <small>{m.width}×{m.height} · {Math.max(1, Math.round(m.bytes / 1024))} KB</small>
          {!picker && <>
            <label>Image description (alt text)
              <input value={alts[m.id] ?? m.alt_text} maxLength={300} onChange={(e) => setAlts((d) => ({ ...d, [m.id]: e.target.value }))} /></label>
            <div className="st-media-actions">
              <button type="button" disabled={busy || (alts[m.id] ?? m.alt_text) === m.alt_text} onClick={() => void saveAlt(m)}>Save alt text</button>
              <button type="button" onClick={() => void copy(m)}>Copy URL</button>
            </div>
          </>}
        </div>
      </div>)}</div>}
  </div>;
}
