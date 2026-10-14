import {
  type InputHTMLAttributes,
  type ReactNode,
  type Ref,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
  useId,
} from 'react';
import { CircleAlert, Search } from 'lucide-react';
import { cx } from '@/lib/utils/cx';

interface FieldProps {
  /** Visible label. Without one, pass an `aria-label`. */
  label?: string;
  /** Hint below the control. */
  help?: ReactNode;
  /** Error message; marks the control `aria-invalid`. */
  error?: string;
}

interface FieldIds {
  controlId: string;
  describedBy: string | undefined;
}

function useFieldIds(id: string | undefined, help: unknown, error: unknown): FieldIds & { helpId: string; errorId: string } {
  const autoId = useId();
  const controlId = id ?? autoId;
  const helpId = `${controlId}-help`;
  const errorId = `${controlId}-error`;
  const describedBy = [error ? errorId : null, help ? helpId : null].filter(Boolean).join(' ') || undefined;
  return { controlId, describedBy, helpId, errorId };
}

function Field({
  label,
  help,
  error,
  controlId,
  helpId,
  errorId,
  className,
  children,
}: FieldProps & { controlId: string; helpId: string; errorId: string; className?: string; children: ReactNode }) {
  if (!label && !help && !error) return <>{children}</>;
  return (
    <div className={cx('field', className)}>
      {label && (
        <label className="label" htmlFor={controlId}>
          {label}
        </label>
      )}
      {children}
      {error && (
        <p className="error-text" id={errorId}>
          <CircleAlert size={15} aria-hidden="true" className="mt-0.5 shrink-0" />
          <span>{error}</span>
        </p>
      )}
      {help && (
        <p className="help" id={helpId}>
          {help}
        </p>
      )}
    </div>
  );
}

export interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size'>, FieldProps {
  /** md is 44 px; lg is 52 px (sign-in, prominent search). Text is 16 px+ so iOS never zooms. */
  size?: 'md' | 'lg';
  /** Icon at the start of the field. */
  leadingIcon?: ReactNode;
  /** Controls inside the end of the field (clear, show password). */
  end?: ReactNode;
  /** Class for the wrapping `.field` when a label/help/error is shown. */
  fieldClassName?: string;
  ref?: Ref<HTMLInputElement>;
}

export function Input({
  label,
  help,
  error,
  size = 'md',
  leadingIcon,
  end,
  id,
  className,
  fieldClassName,
  ref,
  ...props
}: InputProps) {
  const { controlId, describedBy, helpId, errorId } = useFieldIds(id, help, error);
  const input = (
    <input
      ref={ref}
      id={controlId}
      className={cx(
        'input',
        size === 'lg' && 'input-lg',
        // .input-wrap reserves room for a leading icon; reset it when there is none
        Boolean(end) && !leadingIcon && 'pl-3',
        Boolean(end) && 'pr-12',
        className
      )}
      aria-invalid={error ? true : undefined}
      aria-describedby={describedBy}
      {...props}
    />
  );
  return (
    <Field label={label} help={help} error={error} controlId={controlId} helpId={helpId} errorId={errorId} className={fieldClassName}>
      {leadingIcon || end ? (
        <div className="input-wrap">
          {leadingIcon}
          {input}
          {end && <div className="input-end">{end}</div>}
        </div>
      ) : (
        input
      )}
    </Field>
  );
}

/** Text input with a leading search icon. */
export function SearchInput(props: Omit<InputProps, 'leadingIcon' | 'type'>) {
  return <Input type="search" leadingIcon={<Search size={18} aria-hidden="true" />} {...props} />;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement>, FieldProps {
  ref?: Ref<HTMLSelectElement>;
}

export function Select({ label, help, error, id, className, children, ref, ...props }: SelectProps) {
  const { controlId, describedBy, helpId, errorId } = useFieldIds(id, help, error);
  return (
    <Field label={label} help={help} error={error} controlId={controlId} helpId={helpId} errorId={errorId}>
      <select
        ref={ref}
        id={controlId}
        className={cx('input', className)}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        {...props}
      >
        {children}
      </select>
    </Field>
  );
}

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement>, FieldProps {
  ref?: Ref<HTMLTextAreaElement>;
}

export function Textarea({ label, help, error, id, className, ref, ...props }: TextareaProps) {
  const { controlId, describedBy, helpId, errorId } = useFieldIds(id, help, error);
  return (
    <Field label={label} help={help} error={error} controlId={controlId} helpId={helpId} errorId={errorId}>
      <textarea
        ref={ref}
        id={controlId}
        className={cx('input', className)}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        {...props}
      />
    </Field>
  );
}

/** Keyboard key, e.g. ⌘K. */
export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return <kbd className={cx('kbd', className)}>{children}</kbd>;
}
