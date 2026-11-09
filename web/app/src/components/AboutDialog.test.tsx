import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AboutDialog, uiBuild } from './AccountMenu';
const metadata = { libraries: { c2j: { module: 'c2j', version: 'v1.2.3', commit: null }, jobdb: { module: 'jobdb', version: null, commit: 'abc123' } } };
afterEach(() => vi.unstubAllGlobals());
describe('About metadata', () => {
 it('keeps UI visible during loading and independently displays unknown fields', async () => {
  let resolve!: (value: unknown) => void;
  vi.stubGlobal('fetch', vi.fn(() => new Promise(value => { resolve = value; })));
  render(<AboutDialog onClose={() => {}} />);
  expect(screen.getByText('Cortex UI')).toBeVisible();
  expect(screen.getAllByText('Loading…')).toHaveLength(4);
  if (uiBuild.commit) expect(screen.getByText(uiBuild.commit)).toBeVisible();
  await act(async () => resolve({ ok: true, json: async () => metadata }));
  expect(screen.getByText('v1.2.3')).toBeVisible();expect(screen.getByText('abc123')).toBeVisible();
  expect(screen.getAllByText('Unavailable').length).toBeGreaterThanOrEqual(2);
 });
 it.each(['404', 'malformed', 'network'])('retains UI and retries after %s', async mode => {
  const fetch = vi.fn().mockImplementationOnce(() => mode === 'network' ? Promise.reject(Error()) : Promise.resolve({ ok: mode !== '404', status: 404, json: async () => ({}) })).mockResolvedValue({ ok: true, json: async () => metadata });
  vi.stubGlobal('fetch', fetch);
  render(<AboutDialog onClose={() => {}} />);
  expect(await screen.findByText('Could not load library build information.')).toBeVisible();
  expect(screen.getByText('Cortex UI')).toBeVisible();
  fireEvent.click(screen.getByText('Retry'));
  expect(await screen.findByText('v1.2.3')).toBeVisible();
  expect(document.activeElement?.closest('[role="dialog"]')).not.toBeNull();
 });
 it('aborts obsolete requests on unmount', async () => {
  let signal: AbortSignal | undefined;
  vi.stubGlobal('fetch', vi.fn((_url, options) => {signal=options.signal;return new Promise(() => {});}));
  const view=render(<AboutDialog onClose={() => {}} />);
  await waitFor(() => expect(signal).toBeDefined());view.unmount();expect(signal?.aborted).toBe(true);
 });
});
