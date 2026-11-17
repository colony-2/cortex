import { expect, it } from 'vitest';
import { automaticDocuments, clampReviewPane, reviewPaneDefaults, reviewV2Layout } from './reviewV2Layout';

it('L01 allocates only visible sides and their 8px tracks, leaving the rest to the reader', () => {
  for (const documents of [true, false]) for (const discussion of [true, false]) {
    const l = reviewV2Layout(1500, false, documents, discussion, reviewPaneDefaults);
    expect(l.stacked).toBe(false);
    expect(l.documents).toBe(documents ? 240 : 0);
    expect(l.discussion).toBe(discussion ? 320 : 0);
    expect(l.dividers).toBe((Number(documents) + Number(discussion)) * 8);
    expect(l.reader + l.dividers + l.documents + l.discussion).toBe(1500);
  }
});
it('L02 clamps extreme drags without collapsing or changing the opposite side', () => {
  const l = reviewV2Layout(1400, false, true, true, reviewPaneDefaults);
  for (const side of ['documents', 'discussion'] as const) {
    const other = side === 'documents' ? 'discussion' : 'documents';
    expect(clampReviewPane(side, -10000, l.maximum[side])).toBe(side === 'documents' ? 180 : 240);
    const maximum = clampReviewPane(side, 10000, l.maximum[side]);
    const resized = reviewV2Layout(1400, false, true, true, { ...reviewPaneDefaults, [side]: maximum });
    expect(resized.reader).toBe(360);
    expect(resized[other]).toBe(l[other]);
  }
});
it('L03 clamps effective widths on shrink, prioritizes documents, and restores preferences', () => {
  const preferred = Object.freeze({ documents: 420, discussion: 500 });
  expect(reviewV2Layout(1000, false, true, true, preferred)).toMatchObject({ documents: 384, discussion: 240, reader: 360 });
  expect(reviewV2Layout(1200, false, true, true, preferred)).toMatchObject({ documents: 420, discussion: 404, reader: 360 });
  expect(reviewV2Layout(1600, false, true, true, preferred)).toMatchObject(preferred);
  expect(preferred).toEqual({ documents: 420, discussion: 500 });
});
it('L04 calculates the automatic navigation threshold from stable outer width and visible discussion', () => {
  expect(automaticDocuments(1295, true)).toBe(false);
  expect(automaticDocuments(1296, true)).toBe(true);
  expect(automaticDocuments(967, false)).toBe(false);
  expect(automaticDocuments(968, false)).toBe(true);
  // Explicit visibility bypasses the automatic default in the caller.
  expect(reviewV2Layout(1000, false, true, true, reviewPaneDefaults).documents).toBeGreaterThan(0);
});
it('L05 stacks below the viewport breakpoint or visible minimums, retaining the caller preferences', () => {
  expect(reviewV2Layout(796, false, true, true, reviewPaneDefaults).stacked).toBe(false);
  expect(reviewV2Layout(795, false, true, true, reviewPaneDefaults).stacked).toBe(true);
  expect(reviewV2Layout(1500, true, true, true, reviewPaneDefaults).stacked).toBe(true);
  expect(reviewV2Layout(1500, false, true, true, reviewPaneDefaults)).toMatchObject(reviewPaneDefaults);
});
