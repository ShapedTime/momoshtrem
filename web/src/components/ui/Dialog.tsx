'use client';

import type { ReactNode } from 'react';
import * as AlertDialog from '@radix-ui/react-alert-dialog';
import { useRestoreFocus } from '@/hooks/useRestoreFocus';
import { Button } from './Button';

export interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Says exactly what happens: "Remove Severance from your library?" */
  title: string;
  /** What goes and what stays. */
  description: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  /** danger: the only place a solid red button appears. */
  tone?: 'danger' | 'primary';
  onConfirm: () => void | Promise<void>;
  /** Busy state while the confirmed action runs. */
  confirming?: boolean;
  /** Extra content, e.g. an "Also delete downloaded data" checkbox. */
  children?: ReactNode;
}

/**
 * Confirmations only (HANDOFF §4.4). Centred everywhere; on phone the buttons
 * stack with Cancel nearest the thumb. Cancel has initial focus.
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel = 'Cancel',
  tone = 'danger',
  onConfirm,
  confirming = false,
  children,
}: ConfirmDialogProps) {
  const focus = useRestoreFocus();
  return (
    <AlertDialog.Root open={open} onOpenChange={onOpenChange}>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className="scrim z-(--z-dialog)" />
        <AlertDialog.Content className="dialog" {...focus}>
          <AlertDialog.Title className="t-title-3">{title}</AlertDialog.Title>
          <AlertDialog.Description asChild>
            <div className="text-fg-2">{description}</div>
          </AlertDialog.Description>
          {children}
          <div className="actions">
            <AlertDialog.Cancel asChild>
              <Button variant="secondary">{cancelLabel}</Button>
            </AlertDialog.Cancel>
            <Button
              variant={tone}
              loading={confirming}
              onClick={async () => {
                await onConfirm();
              }}
            >
              {confirmLabel}
            </Button>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}
