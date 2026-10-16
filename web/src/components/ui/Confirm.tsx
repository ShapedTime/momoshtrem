'use client';

import { createContext, type ReactNode, useCallback, useContext, useId, useMemo, useRef, useState } from 'react';
import { ConfirmDialog, type ConfirmDialogProps } from './Dialog';

/** An option under the description, e.g. "Also delete downloaded data". */
export interface ConfirmCheckbox {
  label: string;
  defaultChecked?: boolean;
}

export type ConfirmOptions = Pick<ConfirmDialogProps, 'title' | 'description' | 'confirmLabel' | 'cancelLabel' | 'tone'> & {
  checkbox?: ConfirmCheckbox;
};

/** With a checkbox, the answer says how it was left. */
export interface ConfirmAnswer {
  confirmed: boolean;
  checked: boolean;
}

export interface ConfirmFn {
  (options: ConfirmOptions & { checkbox: ConfirmCheckbox }): Promise<ConfirmAnswer>;
  (options: ConfirmOptions): Promise<boolean>;
}

const ConfirmContext = createContext<ConfirmFn | null>(null);

/**
 * `const confirm = useConfirm(); if (await confirm({...})) remove();`
 * One app-wide confirm dialog for destructive actions (HANDOFF §4.4). With a
 * `checkbox`, it resolves `{confirmed, checked}` instead of a boolean.
 */
export function useConfirm(): ConfirmFn {
  const confirm = useContext(ConfirmContext);
  if (!confirm) throw new Error('useConfirm must be used within a ConfirmProvider');
  return confirm;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const [checked, setChecked] = useState(false);
  const resolver = useRef<((confirmed: boolean) => void) | null>(null);
  const checkedRef = useRef(false);
  const checkboxId = useId();

  const confirm = useCallback((next: ConfirmOptions) => {
    resolver.current?.(false);
    const initial = next.checkbox?.defaultChecked ?? false;
    checkedRef.current = initial;
    setChecked(initial);
    setOptions(next);
    return new Promise<boolean | ConfirmAnswer>((resolve) => {
      resolver.current = (confirmed) => resolve(next.checkbox ? { confirmed, checked: checkedRef.current } : confirmed);
    });
  }, []) as ConfirmFn;

  const settle = useCallback((value: boolean) => {
    resolver.current?.(value);
    resolver.current = null;
    setOptions(null);
  }, []);

  const value = useMemo(() => confirm, [confirm]);
  const checkbox = options?.checkbox;

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      {options && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && settle(false)}
          onConfirm={() => settle(true)}
          title={options.title}
          description={options.description}
          confirmLabel={options.confirmLabel}
          cancelLabel={options.cancelLabel}
          tone={options.tone}
        >
          {checkbox && (
            <label htmlFor={checkboxId} className="flex min-h-11 cursor-pointer items-center gap-3">
              <input
                id={checkboxId}
                type="checkbox"
                className="size-5 shrink-0 accent-(--accent)"
                checked={checked}
                onChange={(event) => {
                  checkedRef.current = event.target.checked;
                  setChecked(event.target.checked);
                }}
              />
              <span className="t-small text-fg-2">{checkbox.label}</span>
            </label>
          )}
        </ConfirmDialog>
      )}
    </ConfirmContext.Provider>
  );
}
