import { useState } from 'react';
import { Button, Checkbox, Modal, Space } from 'antd';

export default function CompactFilter({ label, options, selected, onApply }: {
  label: string; options: { value: string; label: string }[]; selected: string[] | null;
  onApply: (values: string[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<string[]>([]);
  return <>
    <Button onClick={() => { setDraft(selected ?? options.map(option => option.value)); setOpen(true); }}>{label}</Button>
    <Modal title={label} open={open} onCancel={() => setOpen(false)} okText="Apply" onOk={() => { onApply(draft); setOpen(false); }}>
      <Space direction="vertical" className="compact-filter">
        <Space wrap><Button onClick={() => setDraft(options.map(option => option.value))}>Select all</Button><Button onClick={() => setDraft([])}>Select none</Button></Space>
        <div className="compact-filter-options">{options.map(option => <Checkbox key={option.value} checked={draft.includes(option.value)} onChange={event => setDraft(previous => event.target.checked ? [...previous, option.value] : previous.filter(value => value !== option.value))}>{option.label}</Checkbox>)}</div>
      </Space>
    </Modal>
  </>;
}
