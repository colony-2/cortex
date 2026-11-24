import { afterEach, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { RecipeJobSummary, ExecutionView } from '@colony2/shared/types';
import { parseJobJSON } from '@colony2/shared/jobJSON';
import JobEnvironment from './JobEnvironment';
import { jobIdentity } from '../test/cellFixtures';

afterEach(cleanup);
const job = (execution?: ExecutionView): RecipeJobSummary => ({ ...jobIdentity, tenant_id: 'tenant', job_id: 'job', status: 'READY', store: 'ACTIVE', job_type: 'recipe', recipe: 'build', created_at: '', available_at: '', execution });
it.each([
  [undefined, 'Execution requirements are unavailable.'],
  ['unspecified', 'No requirements specified.'], ['unresolved', 'Requirements are not yet fully resolved.'],
  ['malformed', 'Invalid execution requirements.'], ['unsupported', 'Unsupported execution requirements.'],
  ['future', 'Execution requirements are unavailable.'],
])('ENV-23 distinguishes %s', (status, text) => {
  render(<JobEnvironment job={job(status ? { status, source: 'absent', published: false } : undefined)} />);
  expect(screen.getByRole('heading', { name: 'Execution requirements' })).toBeVisible();
  expect(screen.getByText(text)).toBeVisible();
});
it('ENV-24,25 shows full escaped values and exact data, with source inside Details', () => {
  const execution = parseJobJSON(`{"status":"specified","source":"yield","published":true,"demand":{"recipe_resolution":"resolved","effective":{"base":"<script>雪/*</script>","resources":{"cpu":"500m","memory":"8Gi"},"capabilities":["docker","network"],"packages":["nix:one","nix:two"],"python":{"requires-python":">=3.12","dependencies":["requests","numpy"]}}},"opaque":9007199254740993}`) as ExecutionView;
  const { container } = render(<JobEnvironment job={{ ...job({ status: 'in_flight', source: 'unavailable', published: false }), recorded_execution: execution }} />);
  expect(screen.getByText('<script>雪/*</script>')).toBeVisible();
  expect(container.querySelector('script')).toBeNull();
  expect(screen.getByText('0.5 cores')).toBeVisible(); expect(screen.getByText('8 GiB')).toBeVisible();
  expect(screen.queryByText('Ephemeral storage')).toBeNull();
  expect(screen.queryByText(/wildcard matches/)).toBeNull();
  expect(screen.getByText('docker')).toBeVisible(); expect(screen.getByText('network')).toBeVisible();
  fireEvent.click(screen.getByText('Packages (2)')); expect(screen.getByText('nix:two')).toBeVisible();
  fireEvent.click(screen.getByText(/Python environment/)); expect(screen.getByText('numpy')).toBeVisible();
  expect(screen.getByText('Source: Published by job')).not.toBeVisible();
  fireEvent.click(screen.getByText('Details'));
  expect(screen.getByText('Source: Published by job')).toBeVisible();
  expect(screen.getByText(/stored requirements do not describe live demand/)).toBeVisible();
  expect(container.querySelector('pre')).toHaveTextContent('9007199254740993');
  expect(screen.queryByText(/last recorded requirements/i)).not.toBeInTheDocument();
});
it('keeps diagnostics as text, never displays invalid values, and clears them on replacement', () => {
  const { rerender, container } = render(<JobEnvironment job={job({ status: 'malformed', source: 'yield', published: false, diagnostic: '<b>坏</b>', demand: { effective: { base: 'ignored' } } })} />);
  fireEvent.click(screen.getByText('Details')); expect(screen.getByText('<b>坏</b>')).toBeVisible();
  expect(container.querySelector('b')).toBeNull(); expect(container.querySelector('dl')).toBeNull();
  rerender(<JobEnvironment job={job({ status: 'unspecified', source: 'submission', published: false })} />);
  expect(screen.queryByText('<b>坏</b>')).not.toBeInTheDocument();
  expect(screen.getByText('No requirements specified.')).toBeVisible();
});
