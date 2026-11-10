import { afterEach, describe, expect, it, vi } from 'vitest';
import { AppShell, readNavigationPreference } from './App';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
const activity = vi.hoisted(() => ({ pendingCount: 0, setCurrentProjectId: vi.fn() }));
vi.mock('@colony2/shared', async importOriginal => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  isAuthenticated: () => true,
  useInputActivity: () => activity,
}));
vi.mock('./hooks/useCompact', () => ({ useCompact: () => false }));
vi.mock('./components/JobsListPage', () => ({ default: () => <input aria-label="Page draft" /> }));
function shell() { return <MemoryRouter initialEntries={['/project/navigation/goals']}><AppShell defaultTenantId="navigation" /></MemoryRouter>; }

describe('desktop navigation preference', () => {
  afterEach(() => { vi.restoreAllMocks(); localStorage.clear(); });
  for (const value of [null, '', 'null', 'undefined', '{}', '1', 'TRUE', 'false', '"true"']) {
    it(`defaults expanded for ${value}`, () => {
      if (value !== null) localStorage.setItem('cortex:desktop-navigation-collapsed', value);
      expect(readNavigationPreference()).toBe(false);
    });
  }
  it('restores a collapsed preference', () => {
    localStorage.setItem('cortex:desktop-navigation-collapsed', 'true');
    expect(readNavigationPreference()).toBe(true);
  });
  it('tolerates denied storage reads', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('denied'); });
    expect(readNavigationPreference()).toBe(false);
  });
});

describe('navigation control and activity', () => {
  afterEach(() => { vi.restoreAllMocks(); localStorage.clear(); activity.pendingCount = 0; });
  it('toggles once for mouse, Enter and Space with accessible state and icons', async () => {
    const user = userEvent.setup();
    render(shell());
    fireEvent.change(screen.getByLabelText("Page draft"), { target: { value: "Unsubmitted draft" } });
    let toggle = screen.getByRole('button', { name: 'Collapse navigation' });
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(toggle.querySelector('[aria-label="menu-fold"]')).toBeTruthy();
    fireEvent.click(toggle);
    toggle = screen.getByRole('button', { name: 'Expand navigation' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(toggle.querySelector('[aria-label="menu-unfold"]')).toBeTruthy();
    toggle.focus(); await user.keyboard('{Enter}');
    expect(screen.getByRole('button', { name: 'Collapse navigation' })).toHaveAttribute('aria-expanded', 'true');
    await user.keyboard(' ');
    expect(screen.getByRole('button', { name: 'Expand navigation' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByLabelText('Page draft')).toHaveValue('Unsubmitted draft');
    expect(document.querySelectorAll('.navigation-toggle')).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Expand navigation' })).toHaveTextContent('');
  });
  it('updates activity descriptions and badge without changing collapsed state', () => {
    const view = render(shell());
    fireEvent.click(screen.getByRole('button', { name: 'Collapse navigation' }));
    for (const count of [0, 7, 120, 3]) {
      activity.pendingCount = count;
      view.rerender(shell());
      expect(screen.getByRole('link', { name: 'Reviews' })).toHaveAccessibleDescription(`${count} pending reviews`);
      expect(screen.getByRole('button', { name: 'Expand navigation' })).toHaveAttribute('aria-expanded', 'false');
    }
  });
  it('keeps rendering and toggling when storage writes fail', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('denied'); });
    render(shell());
    fireEvent.click(screen.getByRole('button', { name: 'Collapse navigation' }));
    expect(screen.getByRole('button', { name: 'Expand navigation' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Expand navigation' }));
    expect(screen.getByRole('button', { name: 'Collapse navigation' })).toBeVisible();
  });
});
