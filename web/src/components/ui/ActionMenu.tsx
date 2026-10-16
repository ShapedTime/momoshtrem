'use client';

import Link from 'next/link';
import { cloneElement, Fragment, type MouseEvent, type ReactElement, type ReactNode, useState } from 'react';
import type { LucideIcon } from 'lucide-react';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import { cx } from '@/lib/utils/cx';
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from './Menu';
import { Sheet } from './Sheet';

export interface ActionMenuItem {
  key: string;
  label: string;
  icon?: LucideIcon;
  /** Right-aligned detail: "2 files", "10". */
  hint?: ReactNode;
  /** Navigate there (closes the menu). */
  href?: string;
  /** Runs once the menu has closed, so a dialog or sheet it opens takes focus. */
  onSelect?: () => void;
  /** Quiet destructive item: muted, red on hover, last. Confirm it yourself (useConfirm). */
  danger?: boolean;
  /** A hairline above it. */
  separator?: boolean;
}

interface ActionMenuProps {
  items: ActionMenuItem[];
  /** The menu's accessible name, and the phone sheet's title unless `sheetTitle` is given. */
  label: string;
  sheetTitle?: ReactNode;
  /** Line under the phone sheet's title, e.g. "28/38 ready · 37.8 GB". */
  sheetDescription?: ReactNode;
  /** The trigger: a button that takes a ref and onClick (IconButton). Focus returns to it on close. */
  children: ReactElement<{ onClick?: (event: MouseEvent<HTMLElement>) => void; 'aria-haspopup'?: 'dialog' | 'menu' }>;
}

/** Defer to after the menu or sheet has closed (and handed focus back to the trigger). */
const later = (action: (() => void) | undefined) => action && window.setTimeout(action, 0);

/**
 * An overflow ("…") menu (HANDOFF §3 Menu): a popover on desktop, a narrow
 * sheet with full-size rows on phone. Keyboard and focus come from Radix:
 * arrows move, Esc closes, and focus returns to the trigger.
 */
export function ActionMenu({ items, label, sheetTitle, sheetDescription, children }: ActionMenuProps) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const [open, setOpen] = useState(false);

  if (desktop) {
    return (
      <Menu>
        <MenuTrigger asChild>{children}</MenuTrigger>
        <MenuContent aria-label={label} className="action-menu">
          {items.map((item) => {
            const Icon = item.icon;
            return (
              <Fragment key={item.key}>
                {item.separator && <MenuSeparator />}
                <MenuItem
                  href={item.href}
                  hint={item.hint}
                  icon={Icon && <Icon size={item.danger ? 16 : 18} aria-hidden="true" />}
                  className={cx(item.danger && 'menu-item--quiet-danger')}
                  onSelect={item.href ? undefined : () => later(item.onSelect)}
                >
                  {item.label}
                </MenuItem>
              </Fragment>
            );
          })}
        </MenuContent>
      </Menu>
    );
  }

  const close = () => setOpen(false);
  return (
    <>
      {cloneElement(children, { onClick: () => setOpen(true), 'aria-haspopup': 'dialog' })}
      <Sheet open={open} onOpenChange={setOpen} title={sheetTitle ?? label} description={sheetDescription} size="narrow">
        <div className="menu menu--sheet">
          {items.map((item) => {
            const Icon = item.icon;
            const content = (
              <>
                {Icon && <Icon size={item.danger ? 18 : 20} aria-hidden="true" />}
                <span>{item.label}</span>
                {item.hint && <span className="hint">{item.hint}</span>}
              </>
            );
            const classes = cx('menu-item', item.danger && 'menu-item--quiet-danger');
            return (
              <Fragment key={item.key}>
                {item.separator && <div className="sep" aria-hidden="true" />}
                {item.href ? (
                  <Link href={item.href} className={classes} onClick={close}>
                    {content}
                  </Link>
                ) : (
                  <button
                    type="button"
                    className={classes}
                    onClick={() => {
                      close();
                      later(item.onSelect);
                    }}
                  >
                    {content}
                  </button>
                )}
              </Fragment>
            );
          })}
        </div>
      </Sheet>
    </>
  );
}
