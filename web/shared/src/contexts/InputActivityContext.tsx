import { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { inputActivityService, type PendingInput } from '../services/inputActivityService';

interface InputActivityContextValue {
  pendingInputs: PendingInput[];
  pendingCount: number;
  isConnected: boolean;
  connectionError: boolean;
  currentProjectId: string | null;
  setCurrentProjectId: (projectId: string | null) => void;
  refresh: () => Promise<void>;
}

const InputActivityContext = createContext<InputActivityContextValue | null>(null);

export interface InputActivityProviderProps {
  children: ReactNode;
}

export function InputActivityProvider({ children }: InputActivityProviderProps) {
  const [currentProjectId, setCurrentProjectIdState] = useState<string | null>(null);
  const [pendingInputs, setPendingInputs] = useState<PendingInput[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [connectionError, setConnectionError] = useState(false);

  const setCurrentProjectId = (projectId: string | null) => {
    setCurrentProjectIdState(projectId);
  };

  const loadPendingInputs = async (projectId: string) => {
    try {
      const inputs = await inputActivityService.getPendingInputs(projectId);
      setPendingInputs(inputs);
    } catch (error) {
      console.error('Failed to load pending inputs:', error);
      setPendingInputs([]);
    }
  };

  const refresh = async () => {
    if (currentProjectId) {
      await loadPendingInputs(currentProjectId);
    }
  };

  useEffect(() => {
    if (!currentProjectId) {
      // No project selected, disconnect
      inputActivityService.disconnect();
      setPendingInputs([]);
      setIsConnected(false);
      return;
    }

    // Connect to SSE for current project
    inputActivityService.connect(currentProjectId);

    // Listen for connection events
    const handleConnected = () => {
      setIsConnected(true);
      setConnectionError(false);
      // Load pending inputs on connection
      loadPendingInputs(currentProjectId);
    };

    const handleDisconnected = () => {
      setIsConnected(false);
    };

    const handleConnectionFailed = () => {
      setConnectionError(true);
      setIsConnected(false);
    };

    const handleInputPending = () => {
      // Reload pending inputs when new input arrives
      loadPendingInputs(currentProjectId);
    };

    const handleInputFinished = () => {
      // Reload when any tab completes or cancels an input request.
      loadPendingInputs(currentProjectId);
    };

    inputActivityService.on('connected', handleConnected);
    inputActivityService.on('disconnected', handleDisconnected);
    inputActivityService.on('connection_failed', handleConnectionFailed);
    inputActivityService.on('input_pending', handleInputPending);
    inputActivityService.on('input_cancelled', handleInputFinished);
    inputActivityService.on('input_completed', handleInputFinished);

    // Load initial data
    if (inputActivityService.getConnectionState() === 'open') {
      setIsConnected(true);
      loadPendingInputs(currentProjectId);
    }

    return () => {
      inputActivityService.off('connected', handleConnected);
      inputActivityService.off('disconnected', handleDisconnected);
      inputActivityService.off('connection_failed', handleConnectionFailed);
      inputActivityService.off('input_pending', handleInputPending);
      inputActivityService.off('input_cancelled', handleInputFinished);
      inputActivityService.off('input_completed', handleInputFinished);
    };
  }, [currentProjectId]);

  const pendingCount = pendingInputs.length;

  return (
    <InputActivityContext.Provider
      value={{
        pendingInputs,
        pendingCount,
        isConnected,
        connectionError,
        currentProjectId,
        setCurrentProjectId,
        refresh,
      }}
    >
      {children}
    </InputActivityContext.Provider>
  );
}

export function useInputActivity() {
  const context = useContext(InputActivityContext);
  if (!context) {
    throw new Error('useInputActivity must be used within InputActivityProvider');
  }
  return context;
}
