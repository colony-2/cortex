import { useEffect, useRef, useState } from 'react';
import {
  APIError,
  providerCapabilities,
  pulseConfig,
  pulseHealth,
  pulseProviders,
  type PulseConfig,
  type PulseHealth,
  type PulseInstances,
  type PulseProvider,
} from '@colony2/shared';

type LegacyProvider = {
  name: string;
  type?: string;
  discovery?: undefined;
  health?: PulseHealth;
  settings?: Record<string, unknown>;
};
export type RunnerProvider = PulseProvider | LegacyProvider;

async function legacyProviders(signal: AbortSignal): Promise<LegacyProvider[]> {
  const [config, health] = await Promise.all([pulseConfig(signal), pulseHealth(signal)]);
  return providerNames(config, health.items).map((name) => {
    const settings = config.providers?.[name];
    return {
      name,
      type: typeof settings?.type === 'string' ? settings.type : undefined,
      health: health.items.find((item) => item.name === name),
      settings,
    };
  });
}

export async function readProviders(signal: AbortSignal): Promise<RunnerProvider[]> {
  try {
    return (await pulseProviders(signal)).items;
  } catch (error) {
    if (!(error instanceof APIError && error.status === 404)) throw error;
    return legacyProviders(signal);
  }
}

export async function readProvider(name: string, signal: AbortSignal): Promise<RunnerProvider> {
  try {
    return await providerCapabilities(name, signal);
  } catch (error) {
    if (!(error instanceof APIError && error.status === 404)) throw error;
    // The inventory distinguishes an unknown provider from an older Pulse controller.
    const legacy = (await readProviders(signal)).find((p) => p.name === name && p.discovery === undefined);
    if (legacy) return legacy;
    throw error;
  }
}

export function useRead<T>(key: string, read: (signal: AbortSignal) => Promise<T>) {
  const loader = useRef(read);
  loader.current = read;
  const [version, setVersion] = useState(0);
  const [state, setState] = useState<{ key: string; value?: T; error?: string; busy: boolean }>();
  useEffect(() => {
    const controller = new AbortController();
    setState((old) => ({ key, value: old?.key === key ? old.value : undefined, busy: true }));
    loader
      .current(controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setState({ key, value, busy: false });
      })
      .catch((e) => {
        if (!controller.signal.aborted)
          setState((old) => ({ key, value: old?.value, busy: false, error: String(e.message || e) }));
      });
    return () => controller.abort();
  }, [key, version]);
  const current = state?.key === key ? state : undefined;
  return {
    busy: current?.busy ?? true,
    value: current?.value,
    error: current?.error,
    refresh: () => setVersion((v) => v + 1),
  };
}

// The owner keys this component by provider or query. Tokens remain opaque,
// including on empty pages; retry resumes from the most recent successful page.
export function usePages<T>(
  read: (signal: AbortSignal, token?: string) => Promise<{ items: T[]; next?: string }>,
) {
  const [pages, setPages] = useState(new Map<string, { items: T[]; next?: string }>());
  const [token, setToken] = useState('');
  const page = useRead(token, (signal) => read(signal, token || undefined));
  useEffect(() => {
    const value = page.value;
    if (value) setPages((previous) => new Map(previous).set(token, value));
  }, [page.value, token]);
  const next = pages.get(token)?.next;
  const repeated = !!next && pages.has(next);
  return {
    items: [...pages.values()].flatMap((page) => page.items),
    busy: page.busy,
    error: page.error || (repeated ? 'Repeated page token.' : undefined),
    retry: page.refresh,
    more: !repeated && next ? () => setToken(next) : undefined,
  };
}

export function providerNames(
  config?: PulseConfig,
  providers?: { name: string }[],
  instances?: PulseInstances,
) {
  return [
    ...new Set([
      ...Object.keys(config?.providers || {}),
      ...(providers || []).map((p) => p.name),
      ...(config?.targets || []).flatMap((t) => (t.launch_services || []).map((s) => s.name)),
      ...(instances?.items || []).map((i) => i.provider),
      ...(instances?.errors || []).map((e) => e.provider),
    ]),
  ].sort();
}
export function displayTime(value?: string) {
  return value && !value.startsWith('0001-') && Number.isFinite(Date.parse(value))
    ? new Date(value).toLocaleString()
    : '—';
}
export function elapsedMinutes(start?: string) {
  if (!start || start.startsWith('0001-')) return undefined;
  const diff = (Date.now() - Date.parse(start)) / 60000;
  return Number.isFinite(diff) && diff >= 0 ? diff : undefined;
}
export function duration(minutes?: number) {
  if (minutes === undefined) return '—';
  if (minutes < 1) return '<1 min';
  const m = Math.floor(minutes);
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)}h ${m % 60}m`;
}

// Public configuration is provider-specific. Keep catalog fields with their
// original labels rather than interpreting controller defaults as supported bases.
export function catalog(settings?: Record<string, unknown>) {
  const entries: [string, unknown][] = [];
  function visit(value: unknown, path: string) {
    if (!value || typeof value !== 'object') return;
    for (const [key, child] of Object.entries(value)) {
      const name = path ? path + '.' + key : key;
      if (
        ['base', 'bases', 'base_labels', 'capability_labels', 'capabilities', 'catalog', 'images'].includes(
          key,
        )
      )
        entries.push([name, child]);
      else visit(child, name);
    }
  }
  visit(settings, '');
  return entries;
}
