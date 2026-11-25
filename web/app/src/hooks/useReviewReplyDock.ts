import { createContext, useContext } from 'react';

export type ReviewReplyTarget = { tenant: string; job: string; reviewId: string; title: string; cellName?: string; returnTo?: string };
export const reviewReplyKey = ({tenant,job,reviewId}: Pick<ReviewReplyTarget, 'tenant' | 'job' | 'reviewId'>) => JSON.stringify([tenant,job,reviewId]);

export const ReviewReplyDock = createContext<{
  open: (target: ReviewReplyTarget) => void;
  has: (id: string) => boolean;
  activate: (id: string) => void;
} | null>(null);

export const useReviewReplyDock = () => useContext(ReviewReplyDock);
