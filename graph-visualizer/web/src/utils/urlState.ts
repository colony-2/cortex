// URL state management utilities

export interface URLState {
  node?: string;      // Selected node ID
  tab?: string;       // Active tab in side panel
  subtab?: string;    // Active subtab (e.g., in changes tab)
}

export function getURLState(): URLState {
  const params = new URLSearchParams(window.location.search);
  return {
    node: params.get('node') || undefined,
    tab: params.get('tab') || undefined,
    subtab: params.get('subtab') || undefined,
  };
}

export function updateURLState(updates: Partial<URLState>) {
  const params = new URLSearchParams(window.location.search);
  
  // Update or remove parameters
  Object.entries(updates).forEach(([key, value]) => {
    if (value) {
      params.set(key, value);
    } else {
      params.delete(key);
    }
  });
  
  // Update URL without reloading the page
  const newURL = `${window.location.pathname}${params.toString() ? '?' + params.toString() : ''}`;
  window.history.replaceState({}, '', newURL);
}

export function clearURLState() {
  window.history.replaceState({}, '', window.location.pathname);
}