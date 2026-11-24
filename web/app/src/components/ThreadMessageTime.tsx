import { formatInboxUpdate } from '../utils/jobInboxData';
import { recordedTime } from './conversationPresentation';
import { Tooltip } from 'antd';

export default function ThreadMessageTime({ value, reported = false }: { value?: unknown; reported?: boolean }) {
  if (typeof value !== 'string') return null;
  const time = recordedTime({ present: true, raw: value });
  if (!time.title) return null;
  const display = formatInboxUpdate(value, new Date());
  return <Tooltip title={reported ? `Reported time: ${time.title}` : time.title}>
    <time className="thread-message-time" dateTime={value}>{display}</time>
  </Tooltip>;
}
