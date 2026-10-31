import { Grid } from 'antd';

export function useCompact() {
  const screens = Grid.useBreakpoint();
  // Ant Design publishes its first measurement after mount. Use the browser's
  // media query initially so phones never mount the desktop document preview.
  return screens.md === undefined
    ? typeof window !== 'undefined' && window.matchMedia('(max-width: 767px)').matches
    : screens.md === false;
}
