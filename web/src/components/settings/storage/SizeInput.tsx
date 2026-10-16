interface SizeInputProps {
  id: string;
  value: string;
  onChange: (text: string) => void;
  placeholder?: string;
  error: string | null;
  describedBy: string;
}

/** A size in GB (lib/settings/storage `parseSize`): the unit shows inside the field until one is typed. */
export function SizeInput({ id, value, onChange, placeholder, error, describedBy }: SizeInputProps) {
  const showUnit = value.trim() !== '' && !/[a-z]/i.test(value);
  return (
    <div className="set-size">
      <input
        id={id}
        className="input"
        type="text"
        inputMode="decimal"
        autoComplete="off"
        spellCheck={false}
        placeholder={placeholder}
        value={value}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        onChange={(event) => onChange(event.target.value)}
      />
      {showUnit && <span aria-hidden="true">GB</span>}
    </div>
  );
}
