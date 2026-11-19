import { describe, expect, it } from 'vitest';
import type { ExecutionView } from '@colony2/shared/types';
import { cpuCores, executionRequirements, memoryQuantity, requirementSummary } from './executionRequirements';

const job = (effective: unknown, status = 'specified') => ({ execution: { status, source: 'submission', published: false, demand: { effective } } });
describe('ENV-08–13 requirement formatting', () => {
  it('formats combined requirements', () => expect(requirementSummary(job({ base: 'macOS-26-arm', resources: { cpu: '4', memory: '8Gi' }, capabilities: ['docker', 'network'] }))).toBe('req: macOS-26-arm 4c/8g [docker, network]'));
  it.each([
    [{ base: '*' }, 'req: *'], [{ resources: { cpu: '500m' } }, 'req: 0.5c'],
    [{ resources: { memory: '512Mi' } }, 'req: 512MiB'], [{ resources: { memory: '8G' } }, 'req: 8GB'],
    [{ capabilities: ['雪', 'network'] }, 'req: [雪, network]'],
    [{ packages: ['one', 'two'] }, 'req: 2 packages'], [{ python: { dependencies: [] } }, 'req: Python'],
    [{ resources: { 'ephemeral-storage': '20Gi' } }, 'req: storage: 20GiB'],
    [{ image: 'alpine', platform: 'linux/amd64' }, 'req: legacy image: alpine legacy platform: linux/amd64'],
    [{ base: 'linux', packages: ['one'], python: {}, resources: { 'ephemeral-storage': '20Gi' }, image: 'legacy', platform: 'linux' }, 'req: linux +5 groups'],
  ])('omits missing parts: %j', (effective, summary) => expect(requirementSummary(job(effective))).toBe(summary));
  it.each([{}, { resources: {} }, { capabilities: [], packages: [] }])('omits empty %j', effective => expect(requirementSummary(job(effective))).toBeUndefined());
  it.each([
    [{ resources: { cpu: 4 } }], [{ capabilities: ['docker', {}] }], [{ base: {} }], [{ resources: [] }],
    [{ python: { dependencies: [4] } }], [{ unknown: 'future' }], [[]], [null],
  ])('does not invent values for unexpected shapes %j', effective => {
    expect(requirementSummary(job(effective))).toBeUndefined();
    expect(executionRequirements(job(effective)).message).toContain('unavailable');
  });
  it.each(['malformed', 'unsupported', 'unknown', 'unspecified'])('omits values for status %s', status => expect(requirementSummary(job({ base: 'ignored' }, status))).toBeUndefined());
  it('shows validated known values while unresolved and never falls back to initial or raw metadata', () => {
    expect(requirementSummary(job({ resources: { cpu: '2' } }, 'unresolved'))).toBe('req: 2c');
    expect(requirementSummary({ execution: { status: 'unresolved', source: 'absent', published: false, initial: { effective: { base: 'ignored' } } } })).toBeUndefined();
  });
  it('uses only the separate retained view for running and finished jobs', () => {
    const execution: ExecutionView = { status: 'in_flight', source: 'unavailable', published: false, demand: { effective: { base: 'old' } } };
    const recorded_execution = job({ base: 'new' }).execution;
    expect(requirementSummary({ execution, recorded_execution })).toBe('req: new');
    expect(requirementSummary({ execution })).toBeUndefined();
    expect(requirementSummary({ execution: { ...execution, status: 'not_waiting' }, recorded_execution })).toBe('req: new');
    expect(requirementSummary({ execution: job({}).execution, recorded_execution })).toBeUndefined();
  });
  it('retains exact decimal quantities and distinguishes units', () => {
    expect(cpuCores('9007199254740993m')).toBe('9007199254740.993');
    expect(cpuCores('1500m')).toBe('1.5'); expect(cpuCores('1.25')).toBe('1.25');
    expect(cpuCores('0.25')).toBe('0.25');
    expect(memoryQuantity('9007199254740993k', true)).toBe('9007199254740993kB');
    expect(memoryQuantity('1.50Gi')).toBe('1.50 GiB'); expect(memoryQuantity('1.50G')).toBe('1.50 GB');
    expect(memoryQuantity('512Mi', true)).toBe('512MiB');
  });
});
