import { useState } from 'react';
import { Button, Checkbox, Modal, Space } from 'antd';

export default function CompactFilter({ label, options, selected, onApply, onApplySelection }: {
  label: string; options: { value: string; label: string }[]; selected: string[] | null;
  onApply?: (values: string[]) => void;
  onApplySelection?: (values: string[] | null) => void;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<string[] | null>(null);
  return <>
    <Button onClick={() => { setDraft(selected); setOpen(true); }}>{label}</Button>
    <Modal title={label} open={open} onCancel={() => setOpen(false)} okText="Apply" onOk={() => { onApply?.(draft ?? options.map(option => option.value)); onApplySelection?.(draft); setOpen(false); }}>
      <Space direction="vertical" className="compact-filter">
        <Space wrap><Button onClick={() => setDraft(null)}>Select all</Button><Button onClick={() => setDraft([])}>Select none</Button></Space>
        <div className="compact-filter-options">{options.map(option => <Checkbox key={option.value} checked={draft === null || draft.includes(option.value)} onChange={event => setDraft(previous => {
          const values = previous ?? options.map(item => item.value);
          return event.target.checked ? [...values, option.value] : values.filter(value => value !== option.value);
        })}>{option.label}</Checkbox>)}</div>
      </Space>
    </Modal>
  </>;
}
