import { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { inputActivityService, type PendingInput, type InputEvent } from '../services/inputActivityService';

interface InputActivityContextValue {
  pendingInputsByCellId: Map<string, PendingInput[]>;
  isConnected: boolean;
  connectionError: boolean;
}

const InputActivityContext = createContext<InputActivityContextValue | null>(null);

export interface InputActivityProviderProps {
  children: ReactNode;
}

export function InputActivityProvider({ children }: InputActivityProviderProps) {
  const [pendingInputsByCellId, setPendingInputsByCellId] = useState<Map<string, PendingInput[]>>(new Map());
  const [isConnected, setIsConnected] = useState(false);
  const [connectionError, setConnectionError] = useState(false);

  useEffect(() => {
    // Connect to SSE on mount
    inputActivityService.connect();

    // Listen for connection events
    const handleConnected = () => {
      setIsConnected(true);
      setConnectionError(false);
      // Load all pending inputs on connection
      loadAllPendingInputs();
    };

    const handleDisconnected = () => {
      setIsConnected(false);
    };

    const handleConnectionFailed = () => {
      setConnectionError(true);
      setIsConnected(false);
    };

    const handleInputEvent = (event: InputEvent) => {
      // Reload pending inputs for the affected cell
      loadPendingInputsForCell(event.cellId);
    };

    inputActivityService.on('connected', handleConnected);
    inputActivityService.on('disconnected', handleDisconnected);
    inputActivityService.on('connection_failed', handleConnectionFailed);
    inputActivityService.on('input_event', handleInputEvent);

    // Initial connection if not already connected
    if (inputActivityService.getConnectionState() === 'closed') {
      inputActivityService.connect();
    } else if (inputActivityService.getConnectionState() === 'open') {
      setIsConnected(true);
      loadAllPendingInputs();
    }

    return () => {
      inputActivityService.off('connected', handleConnected);
      inputActivityService.off('disconnected', handleDisconnected);
      inputActivityService.off('connection_failed', handleConnectionFailed);
      inputActivityService.off('input_event', handleInputEvent);
    };
  }, []);

  const loadAllPendingInputs = async () => {
    try {
      const allInputs = await inputActivityService.getPendingInputs();
      
      // Group inputs by cellId
      const inputsByCellId = new Map<string, PendingInput[]>();
      allInputs.forEach(input => {
        const cellInputs = inputsByCellId.get(input.cellId) || [];
        cellInputs.push(input);
        inputsByCellId.set(input.cellId, cellInputs);
      });
      
      setPendingInputsByCellId(inputsByCellId);
    } catch (error) {
      console.error('Failed to load all pending inputs:', error);
    }
  };

  const loadPendingInputsForCell = async (cellId: string) => {
    try {
      const inputs = await inputActivityService.getPendingInputs(cellId);
      
      setPendingInputsByCellId(prev => {
        const newMap = new Map(prev);
        if (inputs.length === 0) {
          newMap.delete(cellId);
        } else {
          newMap.set(cellId, inputs);
        }
        return newMap;
      });
    } catch (error) {
      console.error(`Failed to load pending inputs for cell ${cellId}:`, error);
    }
  };

  return (
    <InputActivityContext.Provider value={{ pendingInputsByCellId, isConnected, connectionError }}>
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

export function useInputActivityForCell(cellId: string): {
  pendingInputs: PendingInput[];
  pendingCount: number;
  urgency: 'pending' | 'urgent' | 'overdue' | null;
} {
  const { pendingInputsByCellId } = useInputActivity();
  const pendingInputs = pendingInputsByCellId.get(cellId) || [];
  const pendingCount = pendingInputs.filter(i => i.status === 'pending').length;
  
  // Calculate urgency based on expiration times
  let urgency: 'pending' | 'urgent' | 'overdue' | null = null;
  if (pendingCount > 0) {
    const now = new Date().getTime();
    let hasOverdue = false;
    let hasUrgent = false;
    
    pendingInputs.forEach((input: PendingInput) => {
      if (input.status === 'pending') {
        const expiresAt = new Date(input.expiresAt).getTime();
        const timeRemaining = expiresAt - now;
        
        if (timeRemaining <= 0) {
          hasOverdue = true;
        } else if (timeRemaining <= 5 * 60 * 1000) { // 5 minutes
          hasUrgent = true;
        }
      }
    });
    
    if (hasOverdue) {
      urgency = 'overdue';
    } else if (hasUrgent) {
      urgency = 'urgent';
    } else {
      urgency = 'pending';
    }
  }
  
  return { pendingInputs, pendingCount, urgency };
}