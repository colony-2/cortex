import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Badge, Empty, Tabs } from 'antd';
import { FormOutlined } from '@ant-design/icons';
import type { DependencyCell } from '@vibethis/shared';
import { inputActivityService } from '@vibethis/shared';
import InputFormsTab from './InputFormsTab';

interface SidePanelProps {
  selectedCell: DependencyCell | null;
}

export default function SidePanel({ selectedCell }: SidePanelProps) {
  const { cellId } = useParams<{ cellId?: string }>();
  const [pendingInputCount, setPendingInputCount] = useState(0);

  // Track pending inputs for the current cell
  useEffect(() => {
    const effectiveCellId = selectedCell?.id || cellId;
    if (!effectiveCellId) {
      setPendingInputCount(0);
      return;
    }

    inputActivityService
      .getPendingInputs(effectiveCellId)
      .then((inputs) => {
        const pending = inputs.filter((i) => i.status === 'pending');
        setPendingInputCount(pending.length);
      })
      .catch((err) => {
        console.error('Failed to load pending inputs count:', err);
      });

    const unsubscribe = inputActivityService.subscribe(effectiveCellId, () => {
      inputActivityService
        .getPendingInputs(effectiveCellId)
        .then((inputs) => {
          const pending = inputs.filter((i) => i.status === 'pending');
          setPendingInputCount(pending.length);
        })
        .catch((err) => {
          console.error('Failed to update pending inputs count:', err);
        });
    });

    return unsubscribe;
  }, [selectedCell?.id, cellId]);

  if (!cellId && !selectedCell) {
    return (
      <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Empty description="Select a cell to view inputs" />
      </div>
    );
  }

  const items = [
    {
      key: 'inputs',
      label: (
        <span>
          <FormOutlined />
          Inputs
          {pendingInputCount > 0 && <Badge count={pendingInputCount} style={{ marginLeft: 8 }} />}
        </span>
      ),
      children: <InputFormsTab cell={selectedCell} cellId={cellId} />,
    },
  ];

  return (
    <div style={{ height: '100%', background: '#fff', display: 'flex', flexDirection: 'column' }}>
      <Tabs activeKey="inputs" items={items} style={{ flex: 1 }} tabBarStyle={{ marginBottom: 0, paddingLeft: '16px', paddingRight: '16px' }} />
    </div>
  );
}
