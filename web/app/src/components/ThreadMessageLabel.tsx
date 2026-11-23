import { Tag } from 'antd';

const colors = {
  Review: 'purple',
  Feedback: 'blue',
  Question: 'gold',
  Answer: 'cyan',
  Merge: 'geekblue',
  Completion: 'green',
  Failure: 'red',
  Cancellation: 'default',
};

export type MessageLabel = keyof typeof colors;

export function reviewContributionLabel(direction?: { continues_review?: unknown }): MessageLabel {
  return direction?.continues_review === true ? 'Question' : 'Feedback';
}

export default function ThreadMessageLabel({ label }: { label: MessageLabel }) {
  return <Tag className="thread-message-label" color={colors[label]}>{label}</Tag>;
}
