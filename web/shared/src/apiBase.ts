// Use the same origin in production and the Vite proxy in development.
export const API_BASE = import.meta.env.VITE_CORTEX_API_BASE || '/api';
