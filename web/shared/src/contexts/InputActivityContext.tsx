import { createContext, useContext, useState, useEffect, useCallback, useSyncExternalStore, useRef, ReactNode } from 'react';
import { inputActivityService, type UserInputDetails } from '../services/inputActivityService';

const subscribe = (listener: () => void) => {
  inputActivityService.on('discovery_changed', listener);
  inputActivityService.on('details_changed', listener);
  return () => { inputActivityService.off('discovery_changed', listener); inputActivityService.off('details_changed', listener); };
};
export function useInputDiscovery(project: string) {
  return useSyncExternalStore(subscribe, () => inputActivityService.getDiscovery(project));
}
export function useCurrentInput(project: string, job: string, enabled = true) {
  const discovery = useInputDiscovery(project);
  const key = enabled ? inputActivityService.selectedVersion(project, job) : undefined;
  const [turn, setTurn] = useState(0);
  const [result, setResult] = useState<{key:string; details?:UserInputDetails|null; error?:string}>();
  const retained = useRef<{owner: string; key: string; details: UserInputDetails}>();
  const owner = JSON.stringify([project, job]);
  useEffect(() => {
    const changed = () => setTurn(value => value + 1);
    inputActivityService.on('details_changed', changed);
    return () => inputActivityService.off('details_changed', changed);
  }, []);
  useEffect(() => {
    if (!key) return;
    let active = true;
    inputActivityService.getInputDetails(project, job).then(details => {if(active)setResult({key,details});}, error => {if(active)setResult({key,error: error instanceof Error ? error.message : String(error)});});
    return () => {active = false;};
  }, [project, job, key, turn]);
  const state = key ? inputActivityService.getDetailsState(project, job) : undefined;
  const observed = state ?? (result?.key === key ? result : undefined);
  if (observed?.details) retained.current = {owner, key: key!, details: observed.details};
  // Keep reviews readable through normal gaps. An ordinary prompt must disappear
  // on confirmed absence, and a later outage must not bring that obsolete form back.
  const retainedForm = retained.current?.details.form;
  const retainedReview = retainedForm?.kind === 'review' || retainedForm?.presentation?.type === 'review/v2';
  if (enabled && discovery.status === 'current' && (!key || observed?.details === null) && !retainedReview) {
    retained.current = undefined;
  }
  const refresh = useCallback(async () => {
    try {
      const details = await inputActivityService.getInputDetails(project, job, true);
      const key = inputActivityService.selectedVersion(project, job);
      if (key) setResult({key,details});
    } catch (error) {
      const key = inputActivityService.selectedVersion(project, job);
      if (key) setResult({key,error: error instanceof Error ? error.message : String(error)});
      throw error;
    }
  }, [project, job]);
  return { discovery, version: key, details: key ? observed?.details : undefined,
    readableDetails: retained.current?.owner === owner ? retained.current.details : undefined,
    readableVersion: retained.current?.owner === owner ? retained.current.key : undefined,
    error: state ? state.error?.message : result?.key === key ? result?.error : undefined, loading: !!key && (!observed || (observed.details === undefined && !observed.error)), refresh };
}

function useActivityValue() {
  const [currentProjectId, setCurrentProjectId] = useState<string | null>(null);
  const discovery = useInputDiscovery(currentProjectId ?? '');
  const [isConnected, setConnected] = useState(false);
  useEffect(() => {
    const connected = () => setConnected(true);
    const disconnected = () => setConnected(false);
    inputActivityService.on('connected', connected);
    inputActivityService.on('disconnected', disconnected);
    if (currentProjectId) {
      inputActivityService.connect(currentProjectId);
      void inputActivityService.getPendingInputs(currentProjectId, true).catch(() => {});
    } else inputActivityService.disconnect();
    return () => {
      inputActivityService.off('connected', connected); inputActivityService.off('disconnected', disconnected);
      inputActivityService.disconnect();
    };
  }, [currentProjectId]);
  const refresh = useCallback(async () => {
    if (currentProjectId) await inputActivityService.getPendingInputs(currentProjectId, true).catch(() => {});
  }, [currentProjectId]);
  return { pendingInputs: discovery.inputs, pendingCount: discovery.inputs.length, discoveryStatus: discovery.status,
    isConnected, connectionError: discovery.status === 'unavailable', loadError: discovery.error,
    currentProjectId, setCurrentProjectId, refresh };
}
const InputActivityContext = createContext<ReturnType<typeof useActivityValue> | null>(null);
export function InputActivityProvider({children}: {children: ReactNode}) {
  return <InputActivityContext.Provider value={useActivityValue()}>{children}</InputActivityContext.Provider>;
}
export function useInputActivity() {
  const value = useContext(InputActivityContext);
  if (!value) throw new Error('useInputActivity must be used within InputActivityProvider');
  return value;
}
