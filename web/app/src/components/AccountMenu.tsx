import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Dropdown, Modal, Typography } from 'antd';
import { InfoCircleOutlined, LogoutOutlined, UserOutlined } from '@ant-design/icons';
import { getBuildInformation, getUserEmail, type BuildIdentity, type BuildInformation } from '@colony2/shared';
declare const __CORTEX_UI_BUILD__: BuildIdentity;
export const uiBuild = __CORTEX_UI_BUILD__;

export function AboutDialog({ onClose }: { onClose: () => void }) {
  const closeButton = useRef<HTMLButtonElement>(null);
  const [data, setData] = useState<BuildInformation>();
  const [error, setError] = useState<string>();
  const [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError(undefined); setData(undefined);
    getBuildInformation(controller.signal).then(value => {
      if (!controller.signal.aborted) setData(value);
    }).catch(() => {
      if (!controller.signal.aborted) setError('Could not load library build information.');
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [attempt]);
  const row = (name: string, identity?: BuildIdentity, pending = false) => <section className="build-identity" key={name} aria-label={name}>
    <Typography.Title level={5}>{name}</Typography.Title>
    <dl>{(['version', 'commit'] as const).map(field => <div key={field}><dt>{field === 'version' ? 'Version' : 'Commit hash'}</dt><dd>{pending ? 'Loading…' : identity?.[field] || 'Unavailable'}</dd></div>)}</dl>
  </section>;
  return <Modal open title="About cortex" onCancel={onClose} footer={<Button ref={closeButton} onClick={onClose}>Close</Button>}>
    {row('Cortex UI', uiBuild)}
    <Typography.Paragraph type="secondary">Libraries linked into the Cortex API server</Typography.Paragraph>
    {row('c2j library', data?.libraries.c2j, loading)}
    {row('JobDB library', data?.libraries.jobdb, loading)}
    {error && <Alert type="error" showIcon message={error} action={<Button onClick={() => { closeButton.current?.focus(); setAttempt(value => value + 1); }}>Retry</Button>} />}
  </Modal>;
}
export default function AccountMenu({ onLogout }: { onLogout: () => void }) {
  const [open, setOpen] = useState(false);
  const [about, setAbout] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    let frame: number;
    const focusMenu = () => {
      const item = popup.current?.querySelector<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])');
      if (item && item.getClientRects().length) item.focus();
      else frame = requestAnimationFrame(focusMenu);
    };
    // Wait for the popup to be mounted and visible; cancel before a closed
    // popup can steal focus back from the trigger or the About dialog.
    frame = requestAnimationFrame(focusMenu);
    return () => cancelAnimationFrame(frame);
  }, [open]);
  const closeAbout = () => { setAbout(false); requestAnimationFrame(() => button.current?.focus()); };
  return <>
    <Dropdown popupRender={menu => <div ref={popup} onKeyDownCapture={event => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        requestAnimationFrame(() => button.current?.focus());
      }
    }}>{menu}</div>} open={open} onOpenChange={setOpen} trigger={['click']} overlayClassName="account-dropdown" menu={{ items: [
      { key: 'email', label: <span className="account-email">{getUserEmail()}</span>, disabled: true },
      { type: 'divider' },
      { key: 'about', icon: <InfoCircleOutlined aria-hidden />, label: 'About' },
      { key: 'logout', icon: <LogoutOutlined aria-hidden />, label: 'Logout' },
    ], onClick: ({ key }) => { setOpen(false); if (key === 'about') setAbout(true); if (key === 'logout') { setAbout(false); onLogout(); } } }}>
      <Button ref={button} className="account-button" aria-label="User account" aria-haspopup="menu" aria-expanded={open} onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true); } }} icon={<UserOutlined />} />
    </Dropdown>
    {about && <AboutDialog onClose={closeAbout} />}
  </>;
}
