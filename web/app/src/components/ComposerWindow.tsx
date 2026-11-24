import { useState, type ReactNode } from 'react';
import { Button } from 'antd';
import { CloseOutlined, FullscreenExitOutlined, FullscreenOutlined, MinusOutlined, PlusOutlined } from '@ant-design/icons';
import { useCompact } from '../hooks/useCompact';
import { useComposerViewport } from '../hooks/useComposerViewport';

export default function ComposerWindow({ label, title, open = true, active, busy, onActivate, onClose, extra, children }: {
  label: string; title: string; open?: boolean; active: boolean; busy?: boolean;
  onActivate?: () => void; onClose: () => void; extra?: ReactNode;
  children: (collapsed: boolean) => ReactNode;
}) {
  const compact = useCompact();
  const viewportStyle = useComposerViewport();
  const [minimized, setMinimized] = useState(false);
  const [maximized, setMaximized] = useState(false);
  const collapsed = !compact && minimized;
  const close = () => { if (!busy) { setMaximized(false); onClose(); } };
  return <section hidden={!open || (compact && !active)} className={`inbox-composer${collapsed ? ' minimized' : ''}${!compact && maximized && active ? ' maximized' : ''}${active ? ' active' : ''}`}
    aria-label={label} style={viewportStyle} onKeyDown={event => {
      if (event.key !== 'Escape' || event.defaultPrevented) return;
      event.stopPropagation();
      if (!compact && maximized) setMaximized(false); else close();
    }}>
    <header>
      <strong title={title}>{title}</strong>
      {!compact && <>
        {extra}
        <Button type="text" aria-label={collapsed ? 'Expand composer' : 'Minimize composer'} aria-expanded={!collapsed}
          icon={collapsed ? <PlusOutlined /> : <MinusOutlined />} onClick={() => {
            if (collapsed) { onActivate?.(); setMinimized(false); }
            else { setMinimized(true); setMaximized(false); }
          }} />
        <Button type="text" aria-label={maximized ? 'Restore composer size' : 'Maximize composer'}
          icon={maximized ? <FullscreenExitOutlined /> : <FullscreenOutlined />} onClick={() => {
            onActivate?.(); setMinimized(false); setMaximized(value => !value);
          }} />
      </>}
      <Button type="text" aria-label="Close composer" disabled={busy} icon={<CloseOutlined />} onClick={close} />
    </header>
    {children(collapsed)}
  </section>;
}
