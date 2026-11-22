import { useEffect, useState, type CSSProperties } from 'react';

// A keyboard can shrink or pan the visual viewport without resizing the layout
// viewport. Keep the open composer in that visible area without remounting it.
export function useComposerViewport(): CSSProperties {
  const read = () => ({
    '--composer-height': `${window.visualViewport?.height ?? window.innerHeight}px`,
    '--composer-top': `${window.visualViewport?.offsetTop ?? 0}px`,
  } as CSSProperties);
  const [style, setStyle] = useState(read);
  useEffect(() => {
    const viewport = window.visualViewport;
    let frame: number | undefined;
    const update = () => setStyle(read());
    const revealFocus = () => {
      const focused = document.activeElement;
      if (focused instanceof HTMLElement && focused.closest('.new-goal-composer')) {
        focused.scrollIntoView({ block: 'nearest' });
      }
    };
    const resize = () => {
      update();
      if (frame !== undefined) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(revealFocus);
    };
    window.addEventListener('resize', resize);
    viewport?.addEventListener('resize', resize);
    viewport?.addEventListener('scroll', update);
    return () => {
      if (frame !== undefined) cancelAnimationFrame(frame);
      window.removeEventListener('resize', resize);
      viewport?.removeEventListener('resize', resize);
      viewport?.removeEventListener('scroll', update);
    };
  }, []);
  return style;
}
