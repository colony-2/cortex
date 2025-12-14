// URL state management utilities for path-based routing

export interface URLState {
  projectId?: string;
  cellId?: string;     // Selected cell ID
  tab?: string;        // Active tab in side panel
  subtab?: string;     // Active subtab (e.g., in changes tab)
}

// Parse the current URL path to extract state
export function getURLState(): URLState {
  const path = window.location.pathname;
  const parts = path.split('/').filter(Boolean);
  
  const state: URLState = {};
  
  // Parse optional project prefix: /project/:projectId/...
  let offset = 0;
  if (parts[0] === 'project' && parts[1]) {
    state.projectId = parts[1];
    offset = 2;
  }

  // Parse /cell/:cellId/:tab/:subtab
  if (parts[offset] === 'cell' && parts[offset + 1]) {
    state.cellId = parts[offset + 1];
    if (parts[offset + 2]) {
      state.tab = parts[offset + 2];
      if (parts[offset + 3]) {
        state.subtab = parts[offset + 3];
      }
    }
  }
  
  return state;
}

// Navigate to a new path based on state updates
export function navigateToPath(updates: Partial<URLState>) {
  const currentState = getURLState();
  const newState = { ...currentState, ...updates };
  
  let base = '/cells';
  if (newState.projectId) {
    base = `/project/${newState.projectId}/cells`;
  }
  let path = base;
  
  if (newState.cellId) {
    path = newState.projectId
      ? `/project/${newState.projectId}/cell/${newState.cellId}`
      : `/cell/${newState.cellId}`;
    
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

// Clear URL state by navigating to /cells
export function clearURLState() {
  return '/cells';
}
