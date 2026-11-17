import { useEffect, useRef, useState } from 'react';
import { clampReviewPane, reviewPaneMinimums, type ReviewSide } from '../utils/reviewV2Layout';

// Local pointer capture keeps dragging bounded to this workspace without global
// listeners or modifying the document cursor. Geometry never keys pane content.
export default function ReviewPaneSeparator({ side, controls, width, maximum, onResize }: {
  side: ReviewSide; controls: string; width: number; maximum: number; onResize: (width: number) => void;
}) {
  const element = useRef<HTMLDivElement>(null);
  const gesture = useRef<{ id: number; x: number; width: number }>();
  const [dragging, setDragging] = useState(false);
  const change = (value: number) => onResize(clampReviewPane(side, value, maximum));
  const sign = side === 'documents' ? 1 : -1;
  const release = () => {
    const current = gesture.current;
    gesture.current = undefined;
    if (current && element.current?.hasPointerCapture(current.id)) element.current.releasePointerCapture(current.id);
    setDragging(false);
  };
  useEffect(() => () => {
    const current = gesture.current;
    if (current && element.current?.hasPointerCapture(current.id)) element.current.releasePointerCapture(current.id);
    gesture.current = undefined;
  }, []);
  return <div ref={element} role="separator" tabIndex={0} aria-label={`Resize ${side}`} aria-orientation="vertical"
    aria-controls={controls} aria-valuemin={reviewPaneMinimums[side]} aria-valuemax={maximum} aria-valuenow={width}
    aria-valuetext={`${Math.round(width)} pixels`} className={`review-v2-separator${dragging ? ' dragging' : ''}`}
    onKeyDown={event => {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault();
      change(event.key === 'Home' ? reviewPaneMinimums[side] : event.key === 'End' ? maximum : width + (event.key === 'ArrowRight' ? 16 : -16) * sign);
    }}
    onPointerDown={event => {
      if (event.button !== 0 || gesture.current) return;
      event.preventDefault();
      event.currentTarget.focus();
      event.currentTarget.setPointerCapture(event.pointerId);
      gesture.current = { id: event.pointerId, x: event.clientX, width };
      setDragging(true);
    }}
    onPointerMove={event => {
      const current = gesture.current;
      if (current?.id === event.pointerId) change(current.width + (event.clientX - current.x) * sign);
    }}
    onPointerUp={event => { if (gesture.current?.id === event.pointerId) release(); }}
    onPointerCancel={release} onLostPointerCapture={release} />;
}
