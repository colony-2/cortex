import { message } from 'antd';
import { getErrorMessage } from './errorHandling';

export function showError(error: any, fallback: string) {
  const text = getErrorMessage(error, fallback);

  message.error({
    content: <span style={{ whiteSpace: 'pre-wrap' }}>{text}</span>,
    duration: getDurationSeconds(text),
  });
}

function getDurationSeconds(text: string): number {
  if (text.length > 800) return 12;
  if (text.length > 300) return 8;
  return 4;
}

