'use client';
import { Fragment, useState, type ChangeEvent, type KeyboardEvent, type ReactNode } from 'react';

/** Shows <em>…</em> as emphasis without ever injecting HTML. */
export function Emphasis({ text }: { text: string }) {
  const parts = text.split(/(<em>[\s\S]*?<\/em>)/g);
  return <>{parts.map((p, i) => {
    const m = p.match(/^<em>([\s\S]*)<\/em>$/);
    return m ? <em key={i}>{m[1]}</em> : <Fragment key={i}>{p}</Fragment>;
  })}</>;
}

type EditableProps = {
  value: string; label: string; onChange: (v: string) => void;
  multiline?: boolean; html?: boolean; className?: string; maxLength?: number;
};

/**
 * Click-to-edit text drawn in the website's own style. Enter or blur closes
 * the editor; Esc restores the value from before editing began.
 */
export function Editable({ value, label, onChange, multiline, html, className = '', maxLength = 4000 }: EditableProps) {
  const [editing, setEditing] = useState<string | null>(null);
  if (editing !== null) {
    const common = {
      autoFocus: true, value, maxLength, 'aria-label': label, className: `st-input ${className}`,
      onChange: (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => onChange(e.target.value.replace(/\n/g, ' ')),
      onBlur: () => setEditing(null),
      onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
        if (e.key === 'Escape') { e.preventDefault(); onChange(editing); setEditing(null); }
        // The website shows each text as one paragraph, so Enter always finishes editing.
        else if (e.key === 'Enter') { e.preventDefault(); setEditing(null); }
      },
    };
    return <span className="st-editing">
      {multiline ? <textarea rows={Math.min(12, Math.max(3, Math.ceil(value.length / 60)))} {...common} /> : <input {...common} />}
      {html && <small className="st-hint">Wrap words in &lt;em&gt;…&lt;/em&gt; for the italic accent.</small>}
    </span>;
  }
  return <span role="button" tabIndex={0} title={`Edit ${label}`} aria-label={`Edit ${label}`}
    className={`st-editable ${className}${value ? '' : ' st-empty'}`}
    onClick={() => setEditing(value)}
    onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setEditing(value); } }}>
    {value ? (html ? <Emphasis text={value} /> : value) : `+ ${label}`}
  </span>;
}

export function ImageSlot({ src, label, alt, onPick, className = '', children }: { src: string; label: string; alt?: string; onPick: () => void; className?: string; children?: ReactNode }) {
  return <button type="button" className={`st-image ${className}`} onClick={onPick} title={`Change ${label}`}>
    {/* eslint-disable-next-line @next/next/no-img-element -- admin-chosen URLs (library or https) are shown as-is */}
    {src ? <img src={src} alt={alt || ''} /> : <span className="st-image-empty">+ {label}</span>}
    {children}
    <span className="st-image-badge">{src ? 'Change image' : 'Add image'}</span>
  </button>;
}
