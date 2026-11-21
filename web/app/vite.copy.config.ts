import { mergeConfig } from 'vite';
import config from './vite.config';

// Used only by playwright.copy.config.ts; normal dev/production hosts are unchanged.
export default mergeConfig(config, { server: { allowedHosts: ['cortex-copy.test'] } });
