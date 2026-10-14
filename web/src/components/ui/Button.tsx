import Link from 'next/link';
import type { ButtonHTMLAttributes, ComponentProps, Ref } from 'react';
import { cx } from '@/lib/utils/cx';

export type ButtonVariant =
  | 'primary'
  | 'secondary'
  | 'ghost'
  | 'glass'
  | 'ready'
  | 'danger'
  | 'danger-quiet';
export type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonStyleProps {
  /** One `primary` per view: it marks the next thing to do. Solid `danger` lives only in confirm dialogs. */
  variant?: ButtonVariant;
  /** sm is 36 px (44 px hit area), md 44, lg 52. */
  size?: ButtonSize;
  /** Full width. Buttons are never forced full width otherwise. */
  block?: boolean;
  /** Square icon-only button; pass an `aria-label`. */
  iconOnly?: boolean;
}

export function buttonClassName({
  variant = 'secondary',
  size = 'md',
  block = false,
  iconOnly = false,
  loading = false,
  className,
}: ButtonStyleProps & { loading?: boolean; className?: string }): string {
  return cx(
    'btn',
    `btn-${variant}`,
    size !== 'md' && `btn-${size}`,
    block && 'btn-block',
    iconOnly && 'btn-icon',
    loading && 'is-loading',
    className
  );
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement>, ButtonStyleProps {
  /** Shows a spinner and keeps the label for width; the button is busy and inert. */
  loading?: boolean;
  /** @deprecated Use `loading`. */
  isLoading?: boolean;
  ref?: Ref<HTMLButtonElement>;
}

export function Button({
  variant,
  size,
  block,
  iconOnly,
  loading,
  isLoading,
  disabled,
  className,
  type = 'button',
  children,
  ref,
  ...props
}: ButtonProps) {
  const busy = Boolean(loading ?? isLoading);
  return (
    <button
      ref={ref}
      type={type}
      // A busy button keeps its full colour (the spinner says "working"); .is-loading makes it inert
      disabled={disabled}
      aria-busy={busy || undefined}
      onClickCapture={
        busy
          ? (event) => {
              // Keyboard activation still fires click; swallow it (and form submits) while busy
              event.preventDefault();
              event.stopPropagation();
            }
          : undefined
      }
      className={buttonClassName({ variant, size, block, iconOnly, loading: busy, className })}
      {...props}
    >
      {children}
    </button>
  );
}

export interface IconButtonProps extends Omit<ButtonProps, 'iconOnly' | 'children'> {
  /** Required: icon-only buttons are named for screen readers. */
  'aria-label': string;
  children: React.ReactNode;
}

/** Square, icon-only button. `variant` defaults to ghost. */
export function IconButton({ variant = 'ghost', ...props }: IconButtonProps) {
  return <Button variant={variant} iconOnly {...props} />;
}

type ButtonLinkProps = ComponentProps<typeof Link> & ButtonStyleProps;

/** A link that looks like a button (navigation, not an action). */
export function ButtonLink({ variant, size, block, iconOnly, className, ...props }: ButtonLinkProps) {
  return <Link className={buttonClassName({ variant, size, block, iconOnly, className })} {...props} />;
}
