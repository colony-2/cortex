import { useEffect, type RefObject } from 'react';

export function useReviewResponsePane(pane: RefObject<HTMLDivElement>, visible: boolean) {
  useEffect(() => {
    if (!visible || !pane.current) return;
    const element = pane.current;
    const root = document.documentElement;
    const measure = () => {
      const viewport = window.visualViewport;
      const bottom = viewport ? Math.max(0, window.innerHeight - viewport.height - viewport.offsetTop) : 0;
      element.style.maxHeight = `${(viewport?.height ?? window.innerHeight) * .75}px`;
      element.style.bottom = `${bottom}px`;
      root.style.setProperty('--review-action-height', `${element.getBoundingClientRect().height + bottom}px`);
    };
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(measure);
    observer?.observe(element); measure();
    window.addEventListener('resize', measure);
    window.visualViewport?.addEventListener('resize', measure);
    window.visualViewport?.addEventListener('scroll', measure);
    return () => {
      observer?.disconnect(); window.removeEventListener('resize', measure);
      window.visualViewport?.removeEventListener('resize', measure);
      window.visualViewport?.removeEventListener('scroll', measure);
      root.style.removeProperty('--review-action-height');
    };
  }, [pane, visible]);
}
