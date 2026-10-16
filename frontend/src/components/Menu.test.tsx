import { fireEvent, render, screen } from '@testing-library/react';
import { Menu } from './Menu';

describe('Menu', () => {
  it('renders outside clipping containers and runs the selected item', () => {
    const picked: string[] = [];
    const { container } = render(
      // Mimics the title bar: overflow hidden would clip an in-flow dropdown.
      <div style={{ overflow: 'hidden', height: 20 }}>
        <Menu
          trigger={() => <button>Repos</button>}
          items={[
            { key: 'a', label: 'shop', onSelect: () => picked.push('a') },
            { key: 'sep', label: '', separator: true },
            { key: 'add', label: 'Add repository…', onSelect: () => picked.push('add') },
          ]}
        />
      </div>,
    );
    expect(screen.queryByRole('menu')).toBeNull();
    fireEvent.click(screen.getByText('Repos'));
    const menu = screen.getByRole('menu');
    // Portaled to <body>, not inside the clipping container.
    expect(container.contains(menu)).toBe(false);
    expect(document.body.contains(menu)).toBe(true);
    expect(screen.getAllByRole('menuitem')).toHaveLength(2);

    fireEvent.click(screen.getByText('Add repository…'));
    expect(picked).toEqual(['add']);
    expect(screen.queryByRole('menu')).toBeNull();
  });

  it('closes on outside click and Escape', () => {
    render(<Menu trigger={() => <button>Open</button>} items={[{ key: 'x', label: 'X' }]} />);
    fireEvent.click(screen.getByText('Open'));
    expect(screen.getByRole('menu')).toBeInTheDocument();
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole('menu')).toBeNull();

    fireEvent.click(screen.getByText('Open'));
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('menu')).toBeNull();
  });
});
