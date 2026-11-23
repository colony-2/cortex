import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, expect, it } from 'vitest';
import { clearUserEmail, setUserEmail } from '@colony2/shared';
import { useReviewDraft, useReviewDraftRef } from './useReviewDraft';
function Draft({ identity }: { identity: string }) {
  const [text, setText] = useReviewDraft(identity, 'text', '');
  const attempt = useReviewDraftRef<string | undefined>(identity, 'attempt', undefined);
  return <><input aria-label="Draft" value={text} onChange={event => { attempt.current = 'stable-submission'; setText(event.target.value); }} /><output>{attempt.current}</output></>;
}
beforeEach(() => { clearUserEmail(); setUserEmail('one@example.com'); });
it('retains response text and attempt identity across navigation and scopes them to the reader', () => {
  const first = render(<Draft identity="tenant/job/review" />);
  fireEvent.change(screen.getByLabelText('Draft'), { target: { value: 'Keep my response' } });
  first.unmount();
  const second = render(<Draft identity="tenant/job/review" />);
  expect(screen.getByLabelText('Draft')).toHaveValue('Keep my response');
  expect(screen.getByText('stable-submission')).toBeVisible();
  second.unmount();
  const other = render(<Draft identity="other/job/review" />);
  expect(screen.getByLabelText('Draft')).toHaveValue('');
  other.unmount();
  setUserEmail('two@example.com');
  render(<Draft identity="tenant/job/review" />);
  expect(screen.getByLabelText('Draft')).toHaveValue('');
});
it('clears private drafts on logout and keeps anonymous readers isolated', () => {
  const first = render(<Draft identity="tenant/job/review" />);
  fireEvent.change(screen.getByLabelText('Draft'), { target: { value: 'Private' } });
  first.unmount(); clearUserEmail(); setUserEmail('one@example.com');
  const next = render(<Draft identity="tenant/job/review" />);
  expect(screen.getByLabelText('Draft')).toHaveValue('');
  next.unmount(); clearUserEmail();
  const anonymous = render(<Draft identity="tenant/job/review" />);
  fireEvent.change(screen.getByLabelText('Draft'), { target: { value: 'Anonymous' } });
  anonymous.unmount(); render(<Draft identity="tenant/job/review" />);
  expect(screen.getByLabelText('Draft')).toHaveValue('');
});
