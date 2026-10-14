'use client';

import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { CheckCheck, Info, type LucideIcon, TriangleAlert, X } from 'lucide-react';
import { Button, IconButton } from './Button';

export type ToastTone = 'ready' | 'info' | 'error';

export interface ToastOptions {
  tone?: ToastTone;
  title: string;
  body?: string;
  /** At most one action: Undo, Retry, How to play. */
  action?: { label: string; onClick: () => void };
  /** Stay until dismissed. Defaults to true for errors; use it for "ready" moments too. */
  sticky?: boolean;
  icon?: LucideIcon;
}

interface ActiveToast extends ToastOptions {
  id: number;
}

interface ToastContextValue {
  toast: (options: ToastOptions) => number;
  dismiss: (id?: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const AUTO_DISMISS_MS = 5000;
const DEFAULT_ICON: Record<ToastTone, LucideIcon> = { ready: CheckCheck, info: Info, error: TriangleAlert };

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) throw new Error('useToast must be used within a ToastProvider');
  return context;
}

/**
 * One toast at a time; a new one replaces the current (HANDOFF §4.4).
 * Bottom-centre above the tab bar on phone, bottom-right on desktop.
 * Success and info leave after 5 s (paused while hovered or focused);
 * errors and sticky toasts stay until dismissed.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [current, setCurrent] = useState<ActiveToast | null>(null);
  const nextId = useRef(1);

  const dismiss = useCallback((id?: number) => {
    setCurrent((active) => (id === undefined || active?.id === id ? null : active));
  }, []);

  const toast = useCallback((options: ToastOptions) => {
    const id = nextId.current++;
    setCurrent({ ...options, id });
    return id;
  }, []);

  const value = useMemo(() => ({ toast, dismiss }), [toast, dismiss]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-region" aria-live="polite" aria-atomic="true">
        {current && <ToastCard key={current.id} toast={current} onDismiss={() => dismiss(current.id)} />}
      </div>
    </ToastContext.Provider>
  );
}

function ToastCard({ toast, onDismiss }: { toast: ActiveToast; onDismiss: () => void }) {
  const tone = toast.tone ?? 'ready';
  const sticky = toast.sticky ?? tone === 'error';
  const Icon = toast.icon ?? DEFAULT_ICON[tone];
  const [paused, setPaused] = useState(false);

  useEffect(() => {
    if (sticky || paused) return;
    const timer = window.setTimeout(onDismiss, AUTO_DISMISS_MS);
    return () => window.clearTimeout(timer);
  }, [sticky, paused, onDismiss]);

  return (
    <div
      className={`toast toast--${tone}`}
      role={tone === 'error' ? 'alert' : 'status'}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <Icon size={20} aria-hidden="true" />
      <div className="grow">
        <strong>{toast.title}</strong>
        {toast.body && <span className="text-fg-2">{toast.body}</span>}
      </div>
      {toast.action && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            toast.action?.onClick();
            onDismiss();
          }}
        >
          {toast.action.label}
        </Button>
      )}
      <IconButton aria-label="Dismiss" size="sm" onClick={onDismiss}>
        <X size={16} aria-hidden="true" />
      </IconButton>
    </div>
  );
}
