// URL state management utilities for path-based routing

export interface URLState {
  boxId?: string;     // Selected box ID
  tab?: string;       // Active tab in side panel
  subtab?: string;    // Active subtab (e.g., in changes tab)
}

// Parse the current URL path to extract state
export function getURLState(): URLState {
  const path = window.location.pathname;
  const parts = path.split('/').filter(Boolean);
  
  const state: URLState = {};
  
  // Parse /box/:boxId/:tab/:subtab
  if (parts[0] === 'box' && parts[1]) {
    state.boxId = parts[1];
    if (parts[2]) {
      state.tab = parts[2];
      if (parts[3]) {
        state.subtab = parts[3];
      }
    }
  }
  
  return state;
}

// Navigate to a new path based on state updates
export function navigateToPath(updates: Partial<URLState>) {
  const currentState = getURLState();
  const newState = { ...currentState, ...updates };
  
  let path = '/boxes';
  
  if (newState.boxId) {
    path = `/box/${newState.boxId}`;
    
    if (newState.tab) {
      path += `/${newState.tab}`;
      
      if (newState.subtab) {
        path += `/${newState.subtab}`;
      }
    }
  }
  
  // Use React Router's navigation instead of direct history manipulation
  // This will be handled by the component using useNavigate hook
  return path;
}

// Clear URL state by navigating to /boxes
export function clearURLState() {
  return '/boxes';
}