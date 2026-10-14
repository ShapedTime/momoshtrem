'use client';

import type { ComponentProps, ReactNode } from 'react';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { cx } from '@/lib/utils/cx';

/** Popover menu (desktop). On phone, prefer a Sheet with the same items. */
export const Menu = DropdownMenu.Root;
export const MenuTrigger = DropdownMenu.Trigger;

export function MenuContent({ className, sideOffset = 8, align = 'end', ...props }: ComponentProps<typeof DropdownMenu.Content>) {
  return (
    <DropdownMenu.Portal>
      <DropdownMenu.Content
        sideOffset={sideOffset}
        align={align}
        className={cx('menu z-(--z-palette)', className)}
        {...props}
      />
    </DropdownMenu.Portal>
  );
}

interface MenuItemProps extends Omit<ComponentProps<typeof DropdownMenu.Item>, 'children'> {
  icon?: ReactNode;
  /** Right-aligned hint, e.g. a shortcut. */
  hint?: ReactNode;
  /** Quiet red text; destructive items still go through a confirm dialog. */
  danger?: boolean;
  children: ReactNode;
}

export function MenuItem({ icon, hint, danger, className, children, ...props }: MenuItemProps) {
  return (
    <DropdownMenu.Item className={cx('menu-item', danger && 'danger', className)} {...props}>
      {icon}
      <span>{children}</span>
      {hint && <span className="hint">{hint}</span>}
    </DropdownMenu.Item>
  );
}

export function MenuSeparator() {
  return <DropdownMenu.Separator className="sep" />;
}

export function MenuLabel({ className, ...props }: ComponentProps<typeof DropdownMenu.Label>) {
  return <DropdownMenu.Label className={cx('px-3 py-2', className)} {...props} />;
}
