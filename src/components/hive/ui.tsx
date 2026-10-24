'use client';
import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';

/** Polite loading indicator for screen readers and sighted users. */
export function Loading({ label = 'Loading…' }: { label?: string }) {
  return <p className="hv-muted" role="status" aria-live="polite">{label}</p>;
}

/** Error message with an optional retry action. */
export function ErrorNote({ message, onRetry, retryLabel = 'Try again' }: { message: string; onRetry?: () => void; retryLabel?: string }) {
  return (
    <div className="hv-msg err" role="alert">
      <span>{message}</span>
      {onRetry && <button type="button" className="hv-link" onClick={onRetry}>{retryLabel}</button>}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="hv-muted hv-empty">{children}</p>;
}

/** Success/info message announced politely. */
export function Notice({ message, tone = 'ok' }: { message: string; tone?: 'ok' | 'err' }) {
  if (!message) return null;
  return <p className={`hv-msg${tone === 'err' ? ' err' : ''}`} role={tone === 'err' ? 'alert' : 'status'}>{message}</p>;
}

export function Pager({ page, hasMore, onPage, total }: { page: number; hasMore: boolean; onPage: (p: number) => void; total?: number }) {
  if (page === 1 && !hasMore) return null;
  return (
    <nav className="hv-actions hv-pager" aria-label="Pagination">
      <button type="button" className="hv-btn ghost sm" disabled={page <= 1} onClick={() => onPage(page - 1)}>Previous</button>
      <span className="hv-muted">Page {page}{total !== undefined ? ` · ${total} total` : ''}</span>
      <button type="button" className="hv-btn ghost sm" disabled={!hasMore} onClick={() => onPage(page + 1)}>Next</button>
    </nav>
  );
}

type ConfirmOptions = { title: string; body?: string; confirmLabel?: string; cancelLabel?: string; danger?: boolean };
type Pending = ConfirmOptions & { resolve: (ok: boolean) => void };

/**
 * Accessible replacement for window.confirm: a modal dialog with focus moved
 * to the safe (cancel) action, Escape to cancel and focus restored afterwards.
 * Usage: const [confirm, dialog] = useConfirm(); … if (await confirm({...})) …; render {dialog}.
 */
export function useConfirm(): [(o: ConfirmOptions) => Promise<boolean>, ReactNode] {
  const [pending, setPending] = useState<Pending | null>(null);
  const confirm = useCallback((o: ConfirmOptions) => new Promise<boolean>((resolve) => setPending({ ...o, resolve })), []);
  const close = useCallback((ok: boolean) => {
    setPending((p) => { p?.resolve(ok); return null; });
  }, []);
  return [confirm, pending ? <ConfirmDialog key="confirm" {...pending} onClose={close} /> : null];
}

function ConfirmDialog({ title, body, confirmLabel = 'Confirm', cancelLabel = 'Cancel', danger, onClose }: ConfirmOptions & { onClose: (ok: boolean) => void }) {
  const titleId = useId(), bodyId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    cancelRef.current?.focus();
    return () => previous?.focus?.();
  }, []);
  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Escape') { e.preventDefault(); onClose(false); }
    if (e.key === 'Tab') { // keep focus inside the two buttons
      const first = cancelRef.current, last = confirmRef.current;
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus(); }
    }
  }
  return (
    <div className="hv-overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(false); }}>
      <div className="hv-dialog hv-card" role="alertdialog" aria-modal="true" aria-labelledby={titleId} aria-describedby={body ? bodyId : undefined} onKeyDown={onKeyDown}>
        <h2 id={titleId}>{title}</h2>
        {body && <p id={bodyId}>{body}</p>}
        <div className="hv-actions">
          <button ref={cancelRef} type="button" className="hv-btn ghost" onClick={() => onClose(false)}>{cancelLabel}</button>
          <button ref={confirmRef} type="button" className={`hv-btn${danger ? ' danger' : ''}`} onClick={() => onClose(true)}>{confirmLabel}</button>
        </div>
      </div>
    </div>
  );
}
