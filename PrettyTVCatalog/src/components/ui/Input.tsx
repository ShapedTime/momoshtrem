import { InputHTMLAttributes, forwardRef } from 'react';

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  function Input({ label, error, id, className = '', ...props }, ref) {
    const inputId = id || props.name;
    const hasError = Boolean(error);

    return (
      <div className="w-full">
        {label && (
          <label
            htmlFor={inputId}
            className="block text-sm font-medium text-text-secondary mb-2"
          >
            {label}
          </label>
        )}
        <input
          ref={ref}
          id={inputId}
          className={`
            w-full h-12 px-4
            bg-bg-elevated border rounded-md
            text-white placeholder-text-muted
            transition-colors duration-200
            focus:outline-none focus:ring-2 focus:ring-accent-blue focus:border-transparent
            disabled:opacity-50 disabled:cursor-not-allowed
            ${hasError
              ? 'border-red-500 focus:ring-red-500'
              : 'border-border hover:border-text-muted'
            }
            ${className}
          `}
          aria-invalid={hasError}
          aria-describedby={hasError ? `${inputId}-error` : undefined}
          {...props}
        />
        {error && (
          <p
            id={`${inputId}-error`}
            className="mt-2 text-sm text-red-400"
            role="alert"
          >
            {error}
          </p>
        )}
      </div>
    );
  }
);
