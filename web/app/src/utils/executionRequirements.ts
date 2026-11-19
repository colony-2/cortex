import type { ExecutionView, RecipeJobSummary } from '@colony2/shared/types';

export interface Requirements {
  base?: string;
  capabilities?: string[];
  resources?: { cpu?: string; memory?: string; 'ephemeral-storage'?: string };
  packages?: string[];
  python?: { 'requires-python'?: string; dependencies?: string[] };
  image?: string;
  platform?: string;
}
const object = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
const strings = (value: unknown) => Array.isArray(value) && value.every(item => typeof item === 'string');

// Narrow the response boundary, without reinterpreting raw metadata or choosing
// between submission and published snapshots. c2j owns those decisions.
function requirements(value: unknown): Requirements | undefined {
  if (!object(value) || Object.keys(value).some(key => !['base', 'capabilities', 'resources', 'packages', 'python', 'image', 'platform'].includes(key))) return;
  for (const key of ['base', 'image', 'platform']) if (value[key] !== undefined && typeof value[key] !== 'string') return;
  for (const key of ['capabilities', 'packages']) if (value[key] !== undefined && !strings(value[key])) return;
  if (value.resources !== undefined) {
    if (!object(value.resources) || Object.entries(value.resources).some(([key, v]) => !['cpu', 'memory', 'ephemeral-storage'].includes(key) || typeof v !== 'string')) return;
  }
  if (value.python !== undefined) {
    if (!object(value.python) || Object.entries(value.python).some(([key, v]) => key === 'dependencies' ? !strings(v) : key !== 'requires-python' || typeof v !== 'string')) return;
  }
  return value as Requirements;
}

export function executionRequirements(job: Pick<RecipeJobSummary, 'execution' | 'recorded_execution'>): {
  view?: ExecutionView; retained: boolean; values?: Requirements; message?: string;
} {
  const retained = job.execution?.status === 'in_flight' || job.execution?.status === 'not_waiting';
  const view = retained ? job.recorded_execution : job.execution;
  if (!view) return { retained, message: 'Execution requirements are unavailable.' };
  const messages: Record<string, string> = {
    unspecified: 'No requirements specified.', unresolved: 'Requirements are not yet fully resolved.',
    malformed: 'Invalid execution requirements.', unsupported: 'Unsupported execution requirements.',
  };
  if (view.status !== 'specified' && view.status !== 'unresolved' && view.status !== 'unspecified') {
    return { view, retained, message: messages[view.status] || 'Execution requirements are unavailable.' };
  }
  if (view.status === 'unspecified') return { view, retained, message: messages.unspecified };
  const values = object(view.demand) ? requirements(view.demand.effective) : undefined;
  if (!values) return { view, retained, message: view.status === 'unresolved' && !view.demand ? messages.unresolved : 'Execution requirements are unavailable in a recognized format.' };
  return { view, retained, values, message: view.status === 'unresolved' ? messages.unresolved : undefined };
}

// Decimal string arithmetic avoids rounding quantities through JS numbers.
function shiftDecimal(value: string, places: number) {
  const [whole, fraction = ''] = value.replace(/^\+/, '').split('.');
  const digits = whole + fraction;
  const point = whole.length + places;
  const expanded = point <= 0 ? '0.' + '0'.repeat(-point) + digits
    : point >= digits.length ? digits + '0'.repeat(point - digits.length)
    : digits.slice(0, point) + '.' + digits.slice(point);
  return expanded.replace(/^0+(?=\d)/, '').replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');
}
export function cpuCores(value: string) {
  const match = /^(\+?(?:\d+(?:\.\d*)?|\.\d+))([numkKMGTPE]?)$/.exec(value);
  if (!match) return value;
  const power: Record<string, number> = { n: -9, u: -6, m: -3, '': 0, k: 3, K: 3, M: 6, G: 9, T: 12, P: 15, E: 18 };
  return shiftDecimal(match[1], power[match[2]]);
}
export function memoryQuantity(value: string, compact = false) {
  const match = /^(\+?(?:\d+(?:\.\d*)?|\.\d+))([KMGTPE]i|[numkKMGTPE])?$/.exec(value);
  if (!match) return `${value}${compact ? 'B' : ' B'}`;
  const [, amount, unit = ''] = match;
  if (compact && unit === 'Gi') return `${amount}g`;
  const suffix = unit === 'k' ? 'kB' : `${unit}B`;
  return `${amount}${compact ? '' : ' '}${suffix}`;
}
export function requirementSummary(job: Pick<RecipeJobSummary, 'execution' | 'recorded_execution'>) {
  const values = executionRequirements(job).values;
  if (!values) return;
  const primary: string[] = [];
  if (values.base) primary.push(values.base);
  const cpu = values.resources?.cpu;
  const memory = values.resources?.memory;
  const resources = [cpu ? `${cpuCores(cpu)}c` : '', memory ? memoryQuantity(memory, true) : ''].filter(Boolean).join('/');
  if (resources) primary.push(resources);
  if (values.capabilities?.length) primary.push(`[${values.capabilities.join(', ')}]`);
  const extra: string[] = [];
  if (values.packages?.length) extra.push(`${values.packages.length} package${values.packages.length === 1 ? '' : 's'}`);
  if (values.python) extra.push('Python');
  if (values.resources?.['ephemeral-storage']) extra.push(`storage: ${memoryQuantity(values.resources['ephemeral-storage']).replace(' ', '')}`);
  if (values.image) extra.push(`legacy image: ${values.image}`);
  if (values.platform) extra.push(`legacy platform: ${values.platform}`);
  const parts = primary.length ? [...primary, ...(extra.length ? [`+${extra.length} groups`] : [])] : extra;
  return parts.length ? `req: ${parts.join(' ')}` : undefined;
}
