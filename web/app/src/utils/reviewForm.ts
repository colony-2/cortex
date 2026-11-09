import type { InputForm } from '@colony2/shared';

// Presentation convention: use option buttons for a review's sole required choice.
// c2j assigns no special meaning to its field ID or option values.
export function reviewActionField(form: InputForm) {
  const choices = form.fields.filter(field => field.type === 'multiple_choice');
  return form.kind === 'review' && choices.length === 1 && choices[0].required && choices[0].options?.length
    ? choices[0] : undefined;
}

// Only the established two-value convention gets the rejection reason editor.
export function conventionalReviewFeedback(form: InputForm) {
  const action = reviewActionField(form);
  if (action?.options?.length !== 2 || !action.options.includes('approve') || !action.options.includes('revise')) return undefined;
  return form.fields.find(field => field.id === 'feedback' && field.type === 'paragraph_text');
}
