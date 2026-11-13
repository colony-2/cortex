import { useEffect, useRef, useState } from 'react';

// One active refresh per mounted scope. Aborting this HTTP read never sends an
// inference cancellation. Timers pause while hidden; reconnect requests a read.
export function useAssistantPoll(scope: string, enabled: boolean, load: (signal: AbortSignal) => Promise<void>) {
  const callback = useRef(load);
  callback.current = load;
  const [revision, setRevision] = useState(0);
  const refresh = () => setRevision(value => value + 1);
  const wake = useRef<() => void>(() => {});
  useEffect(() => { wake.current(); }, [revision]);
  useEffect(() => {
    if (!enabled) return;
    let stopped = false;
    let active = false;
    let pending = false;
    let failures = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | undefined;
    const run = async () => {
      clearTimeout(timer);
      if (stopped || document.hidden) return;
      if (active) { pending = true; return; }
      active = true;
      pending = false;
      controller = new AbortController();
      try { await callback.current(controller.signal); failures = 0; }
      catch { failures = Math.min(failures + 1, 5); }
      finally {
        active = false;
        if (!stopped && !document.hidden) timer = setTimeout(run, pending ? 0 : Math.min(30_000, 1000 * 2 ** failures));
      }
    };
    const visibility = () => { clearTimeout(timer); if (!document.hidden) void run(); };
    wake.current = () => { void run(); };
    document.addEventListener('visibilitychange', visibility);
    window.addEventListener('online', visibility);
    void run();
    return () => {
      stopped = true;
      clearTimeout(timer);
      controller?.abort();
      wake.current = () => {};
      document.removeEventListener('visibilitychange', visibility);
      window.removeEventListener('online', visibility);
    };
  }, [scope, enabled]);
  return refresh;
}
