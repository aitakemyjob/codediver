import { render, screen, fireEvent } from '@testing-library/react';
import { ViewErrorBoundary } from './ViewHost';

function Boom({ fail }: { fail: boolean }) {
  if (fail) throw new Error('kaboom');
  return <div>fine</div>;
}

describe('ViewErrorBoundary', () => {
  it('contains a crashing view and can retry', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    let fail = true;
    const { rerender } = render(<ViewErrorBoundary title="t"><Boom fail={fail} /></ViewErrorBoundary>);
    expect(screen.getByText('This view hit an error')).toBeInTheDocument();
    expect(screen.getByText('kaboom')).toBeInTheDocument();
    fail = false;
    rerender(<ViewErrorBoundary title="t"><Boom fail={fail} /></ViewErrorBoundary>);
    fireEvent.click(screen.getByText('Try again'));
    expect(screen.getByText('fine')).toBeInTheDocument();
  });
});
