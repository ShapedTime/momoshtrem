import { useState } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { Range } from '@/components/ui/Range';

function Harness({ onChange }: { onChange?: (value: [number, number]) => void }) {
  const [value, setValue] = useState<[number, number]>([20, 80]);
  return (
    <Range
      min={0}
      max={100}
      value={value}
      onValueChange={(next) => {
        setValue(next);
        onChange?.(next);
      }}
      labels={['Minimum size', 'Maximum size']}
      valueText={(n, thumb) => (thumb === 'max' && n === 100 ? 'No maximum' : `${n}%`)}
    />
  );
}

describe('Range', () => {
  it('is two named sliders that read their values', () => {
    render(<Harness />);
    expect(screen.getByRole('slider', { name: 'Minimum size' })).toHaveAttribute('aria-valuetext', '20%');
    expect(screen.getByRole('slider', { name: 'Maximum size' })).toHaveAttribute('aria-valuetext', '80%');
    expect(screen.getByText('20%')).toBeInTheDocument();
  });

  it('moves each thumb, and never lets them cross', () => {
    const onChange = jest.fn();
    render(<Harness onChange={onChange} />);
    fireEvent.change(screen.getByRole('slider', { name: 'Maximum size' }), { target: { value: '100' } });
    expect(onChange).toHaveBeenLastCalledWith([20, 100]);
    expect(screen.getByRole('slider', { name: 'Maximum size' })).toHaveAttribute('aria-valuetext', 'No maximum');
    fireEvent.change(screen.getByRole('slider', { name: 'Minimum size' }), { target: { value: '100' } });
    expect(onChange).toHaveBeenLastCalledWith([100, 100]);
    fireEvent.change(screen.getByRole('slider', { name: 'Maximum size' }), { target: { value: '40' } });
    expect(onChange).toHaveBeenLastCalledWith([100, 100]);
  });
});
