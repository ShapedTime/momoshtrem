'use client';

import type { ReactNode } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { Drawer } from 'vaul';
import { X } from 'lucide-react';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import { useRestoreFocus } from '@/hooks/useRestoreFocus';
import { cx } from '@/lib/utils/cx';
import { IconButton } from './Button';

export interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  /** Small line above the title, part of its accessible name: "Find a source · Interstellar". */
  kicker?: ReactNode;
  /** The close button's name when closing keeps something: "Close (your choices are kept)". */
  closeLabel?: string;
  /** Short line under the title; also the dialog's accessible description. */
  description?: ReactNode;
  /** A quiet action in the title row, before the close button: the Filters sheet's Reset. */
  titleAction?: ReactNode;
  /**
   * For read-only content with nothing to focus (Inspect's file list): the body
   * becomes a labelled region in the tab order, so a keyboard can scroll it.
   */
  bodyLabel?: string;
  /**
   * default: sizes to content. narrow: 480 px panel on desktop.
   * full: opens at full height immediately so loading never changes the height
   * (search sheets), and only the grabber drags on touch.
   */
  size?: 'default' | 'narrow' | 'full';
  /** Stacked on another sheet (filters, inspect, pickers). Esc closes only the top one. */
  nested?: boolean;
  /** Extra sticky head content: search field, chips. */
  head?: ReactNode;
  /** Sticky foot with the primary action (glass, includes the safe area). */
  footer?: ReactNode;
  className?: string;
  children: ReactNode;
}

/**
 * One component, three presentations (HANDOFF §4.1): a bottom sheet below
 * 640 px, a centred bottom sheet up to 1023 px (both Vaul, drag to dismiss),
 * and a right-hand panel from 1024 px (Radix Dialog). Focus is trapped,
 * Esc closes, the page doesn't scroll behind, and focus returns to the opener.
 */
export function Sheet(props: SheetProps) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  return desktop ? <PanelSheet {...props} /> : <DrawerSheet {...props} />;
}

function sheetClass({ size = 'default', nested, className }: Pick<SheetProps, 'size' | 'nested' | 'className'>) {
  return cx('sheet', size === 'full' && 'sheet--full', size === 'narrow' && 'sheet--narrow', nested && 'sheet--nested', className);
}

/** Toasts render outside the sheet: tapping one (its ✕, Undo) mustn't close the sheet too. */
function keepOpenForToasts(event: Event) {
  if (event.target instanceof Element && event.target.closest('.toast-region')) event.preventDefault();
}

function SheetHead({ title, kicker, closeLabel = 'Close', description, titleAction, head, Title, Description, Close }: {
  title: ReactNode;
  kicker?: ReactNode;
  closeLabel?: string;
  description?: ReactNode;
  titleAction?: ReactNode;
  head?: ReactNode;
  Title: typeof Dialog.Title;
  Description: typeof Dialog.Description;
  Close: typeof Dialog.Close;
}) {
  return (
    <div className="sheet-head">
      <div className="sheet-title">
        <div className="flex min-w-0 flex-col gap-0.5">
          <Title className="t-title-3 min-w-0">
            {kicker && (
              <>
                <span className="sheet-kicker">{kicker}</span>{' '}
              </>
            )}
            {title}
          </Title>
          {description && <Description className="t-small text-fg-3">{description}</Description>}
        </div>
        {titleAction && <span className="ml-auto shrink-0">{titleAction}</span>}
        <Close asChild>
          <IconButton aria-label={closeLabel} size="sm" className="-mr-2 shrink-0">
            <X size={20} aria-hidden="true" />
          </IconButton>
        </Close>
      </div>
      {head}
    </div>
  );
}

function SheetBody({ label, children }: { label?: string; children: ReactNode }) {
  return (
    <div className="sheet-body" {...(label ? { role: 'region', 'aria-label': label, tabIndex: 0 } : {})}>
      {children}
    </div>
  );
}

function PanelSheet({ open, onOpenChange, title, kicker, closeLabel, description, titleAction, bodyLabel, size, nested, head, footer, className, children }: SheetProps) {
  const focus = useRestoreFocus({ focusContainer: true });
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        {!nested && <Dialog.Overlay className="scrim" />}
        {/* Without a description, opt out of aria-describedby explicitly (Radix warns otherwise) */}
        <Dialog.Content
          className={cx(sheetClass({ size, nested, className }), 'outline-none')}
          onPointerDownOutside={keepOpenForToasts}
          onInteractOutside={keepOpenForToasts}
          {...focus}
          {...(description ? {} : { 'aria-describedby': undefined })}
        >
          <SheetHead
            title={title}
            kicker={kicker}
            closeLabel={closeLabel}
            description={description}
            titleAction={titleAction}
            head={head}
            Title={Dialog.Title}
            Description={Dialog.Description}
            Close={Dialog.Close}
          />
          <SheetBody label={bodyLabel}>{children}</SheetBody>
          {footer && <div className="sheet-foot">{footer}</div>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function DrawerSheet({ open, onOpenChange, title, kicker, closeLabel, description, titleAction, bodyLabel, size, nested, head, footer, className, children }: SheetProps) {
  const focus = useRestoreFocus({ focusContainer: true });
  const Root = nested ? Drawer.NestedRoot : Drawer.Root;
  return (
    <Root open={open} onOpenChange={onOpenChange} handleOnly={size === 'full'} repositionInputs={false}>
      <Drawer.Portal>
        <Drawer.Overlay className="scrim" />
        <Drawer.Content
          className={cx(sheetClass({ size, nested, className }), 'outline-none')}
          onPointerDownOutside={keepOpenForToasts}
          onInteractOutside={keepOpenForToasts}
          {...focus}
          {...(description ? {} : { 'aria-describedby': undefined })}
        >
          {/* Vaul injects unlayered handle styles, hence the important overrides */}
          <div className="grid h-5 shrink-0 place-items-center">
            <Drawer.Handle className="w-9! bg-surface-4! opacity-100!" />
          </div>
          <SheetHead
            title={title}
            kicker={kicker}
            closeLabel={closeLabel}
            description={description}
            titleAction={titleAction}
            head={head}
            Title={Drawer.Title}
            Description={Drawer.Description}
            Close={Drawer.Close}
          />
          <SheetBody label={bodyLabel}>{children}</SheetBody>
          {footer && <div className="sheet-foot">{footer}</div>}
        </Drawer.Content>
      </Drawer.Portal>
    </Root>
  );
}
