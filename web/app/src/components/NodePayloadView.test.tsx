import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { parseJobJSON } from '@colony2/shared';
import NodePayloadView from './NodePayloadView';
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const mount = (value: unknown) => render(<NodePayloadView value={value} label="Input" identity="one" />);
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));
it('renders ordered own literal fields and nested boundaries without unwrapping envelopes', () => {
  const data = Object.assign(Object.create({ inherited: 4 }), { '': '', 'a.b': { child: ['first', 'second'] }, a: { b: { child: 'independent' } }, response: { result: 1 }, '<b>': '<img src=x onerror=alert(1)>', text: '  line\n next', json: '{"x":1}' });
  const { container } = mount(data);
  expect(container.querySelector('.node-payload-children')?.firstChild).toHaveTextContent('Empty string');
  expect(screen.queryByText('inherited')).toBeNull();
  expect(screen.queryByText('first')).toBeNull();
  click('a.b Object · 1 field');
  click('child Array · 2 items');
  expect(screen.getByText('first').closest('.node-payload-row')).toHaveTextContent('0first');
  expect(screen.getByText('second').closest('.node-payload-row')).toHaveTextContent('1second');
  expect(screen.getByRole('button', { name: 'a Object · 1 field' })).toHaveAttribute('aria-expanded', 'false');
  expect(screen.getByRole('button', { name: /response Object/ })).toHaveAttribute('aria-expanded', 'false');
  expect(container.querySelector('img')).toBeNull();
  expect(container.querySelector('a')).toBeNull();
  expect(screen.getByText('line next').textContent).toBe('  line\n next');
  expect(screen.getByText('{"x":1}')).toBeVisible();
});
it.each([null, false, 0, '', {}, [], undefined])('distinguishes root %j and copies complete data', async value => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  mount(value);
  if (value === undefined) { expect(screen.getByText('No input payload')).toBeVisible(); return; }
  expect(screen.queryByText('No input payload')).toBeNull();
  click('Copy JSON');
  expect(writeText).toHaveBeenCalledWith(JSON.stringify(value, null, 2));
  expect(await screen.findByRole('status')).toHaveTextContent('JSON copied');
  click('JSON');
  expect(screen.getByLabelText('Input JSON').textContent).toBe(JSON.stringify(value, null, 2));
});
it('renders exact lossless numeric leaves and reports clipboard failure', async () => {
  const value = parseJobJSON('{"large":900719925474099312345,"decimal":1.234567890123456789}');
  const before = JSON.stringify(value);
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  mount(value);
  click('Copy JSON');
  expect(writeText.mock.calls[0][0]).toContain('900719925474099312345');
  expect(writeText.mock.calls[0][0]).toContain('1.234567890123456789');
  expect(JSON.stringify(value)).toBe(before);
  expect(screen.getByText('900719925474099312345')).toBeVisible();
  expect(screen.queryByText('isLosslessNumber')).toBeNull();
  click('JSON');
  expect(screen.getByLabelText('Input JSON')).toHaveTextContent('1.234567890123456789');
  await screen.findByText('JSON copied');
  vi.stubGlobal('navigator', {});
  click('Copy JSON');
  expect(await screen.findByRole('status')).toHaveTextContent('Could not copy JSON');
});
it('bounds siblings and previews while keeping complete copying and resetting identity', () => {
  const value = { rows: Array.from({ length: 201 }, (_, i) => `item-${i}`), text: 'x'.repeat(4001) };
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  const view = mount(value);
  click(/rows Array/);
  expect(screen.queryByText('item-100')).toBeNull();
  click('Show 100 more');
  expect(screen.getByText('item-199')).toBeVisible();
  expect(screen.queryByText('item-200')).toBeNull();
  click('Show 100 more');
  expect(screen.getByText('item-200')).toBeVisible();
  expect(screen.getByText(/Preview: first/)).toBeVisible();
  click('Show full');
  expect(screen.getByText('x'.repeat(4001))).toBeVisible();
  click('Show less');
  click('Copy JSON');
  expect(writeText).toHaveBeenCalledWith(JSON.stringify(value, null, 2));
  click('JSON');
  view.rerender(<NodePayloadView value={value} identity="two" label="Input" />);
  expect(screen.getByRole('button', { name: 'Structured' })).toHaveAttribute('aria-pressed', 'true');
  expect(screen.queryByText('item-0')).toBeNull();
  expect(screen.getByRole('button', { name: 'Show full' })).toBeVisible();
});
it('caps recursive mounting with an explicit JSON escape', () => {
  let value: unknown = 'bottom';
  for (let i = 0; i < 35; i++) value = { nested: value };
  mount(value);
  for (let i = 0; i < 30; i++) {
    const buttons = screen.getAllByRole('button', { name: /nested Object/ });
    fireEvent.click(buttons[buttons.length - 1]);
  }
  expect(screen.queryByText('bottom')).toBeNull();
  click(/Nesting limit/);
  expect(screen.getByLabelText('Input JSON')).toHaveTextContent('bottom');
});
it('keeps separate instances independent and handles rejected clipboard promises', async () => {
  vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } });
  render(<><NodePayloadView value={[false, 0]} identity="in" label="Input" /><NodePayloadView value={null} identity="out" label="Output" /></>);
  fireEvent.click(within(screen.getByLabelText('Input payload')).getByRole('button', { name: 'JSON' }));
  expect(within(screen.getByLabelText('Output payload')).getByRole('button', { name: 'Structured' })).toHaveAttribute('aria-pressed', 'true');
  fireEvent.click(within(screen.getByLabelText('Output payload')).getByRole('button', { name: 'Copy JSON' }));
  expect(await screen.findByText('Could not copy JSON')).toBeVisible();
});

it('preserves enumeration order and separates path-like names from nested numeric indexes', () => {
  const { container } = mount({ '10': 'ten', '2': 'two', 'a/0': { inside: 1 }, a: [{ inside: 2 }], 'with spaces': true });
  const root = container.querySelector('.node-payload-children')!;
  expect(Array.from(root.children).map(el => el.textContent)).toEqual(['2two', '10ten', '▸ a/0 Object · 1 field', '▸ a Array · 1 item', 'with spacestrue']);
  click('a/0 Object · 1 field');
  click('a Array · 1 item');
  click('0 Object · 1 field');
  expect(screen.getByText('1')).toBeVisible();
  expect(screen.getByText('2', { selector: '.node-payload-value span' })).toBeVisible();
});
