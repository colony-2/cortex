// Widths belong to the mounted review, not the document or native request.
export const reviewPaneDefaults = { documents: 240, discussion: 320 };
export const reviewPaneMinimums = { documents: 180, discussion: 240, reader: 360 };
export const reviewDividerWidth = 8;
export type ReviewSide = 'documents' | 'discussion';
export type ReviewWidths = Record<ReviewSide, number>;

export function automaticDocuments(width: number, discussion: boolean) {
  return width >= 240 + 720 + 8 + (discussion ? 320 + 8 : 0);
}

export function reviewV2Layout(width: number, narrow: boolean, documents: boolean, discussion: boolean, preferred: ReviewWidths) {
  const dividers = (Number(documents) + Number(discussion)) * reviewDividerWidth;
  const minimum = (documents ? 180 : 0) + (discussion ? 240 : 0) + 360 + dividers;
  const stacked = narrow || width < minimum;
  let surplus = Math.max(0, width - minimum);
  const documentExtra = documents ? Math.min(surplus, Math.max(0, preferred.documents - 180)) : 0;
  surplus -= documentExtra;
  const discussionExtra = discussion ? Math.min(surplus, Math.max(0, preferred.discussion - 240)) : 0;
  const sides = { documents: documents ? 180 + documentExtra : 0, discussion: discussion ? 240 + discussionExtra : 0 };
  return { ...sides, stacked, dividers, reader: Math.max(0, width - dividers - sides.documents - sides.discussion),
    maximum: { documents: width - dividers - 360 - sides.discussion, discussion: width - dividers - 360 - sides.documents } };
}

export function clampReviewPane(side: ReviewSide, value: number, maximum: number) {
  return Math.max(reviewPaneMinimums[side], Math.min(maximum, value));
}
