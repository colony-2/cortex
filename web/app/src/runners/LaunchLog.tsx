import { useEffect, useRef, useState } from 'react';
import { pulseLog, pulseLogDownload, type LogPage } from '@colony2/shared';

import { Button, Typography } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';

const previewSize = 10 * 1024;
function bytes(page: LogPage) {
  return page.records.map((r) => Uint8Array.from(atob(r.data), (c) => c.charCodeAt(0)));
}
function advance(token: string | undefined, page: LogPage, seen: Set<string>) {
  if (
    page.has_more &&
    (!page.next_page_token || token === page.next_page_token || seen.has(page.next_page_token))
  ) {
    throw new Error('Pulse returned a repeated log checkpoint.');
  }
  if (token) seen.add(token);
  return page.next_page_token;
}

export default function LaunchLog({ provider, launch }: { provider: string; launch: string }) {
  const [text, setText] = useState('');
  const [page, setPage] = useState<LogPage>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const stream = useRef({
    token: undefined as string | undefined,
    pending: new Uint8Array(0),
    decoder: new TextDecoder(),
    seen: new Set<string>(),
  });
  const pending = useRef<AbortController>();

  async function load() {
    if (pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError('');
    const cursor = stream.current;
    let remaining = previewSize;
    const append = (data: Uint8Array) => {
      const count = Math.min(remaining, data.length);
      const chunk = cursor.decoder.decode(data.subarray(0, count), { stream: true });
      setText((old) => old + chunk);
      remaining -= count;
      return data.slice(count);
    };
    try {
      cursor.pending = append(cursor.pending);
      if (page?.finalized && !page.has_more && !cursor.pending.length) {
        const tail = cursor.decoder.decode();
        setText((old) => old + tail);
        return;
      }
      let size = 3;
      for (let pages = 0; remaining > 0 && pages < 8; pages++) {
        const part = await pulseLog(provider, launch, controller.signal, size, cursor.token);
        if (controller.signal.aborted) return;
        const data = bytes(part);
        cursor.token = advance(cursor.token, part, cursor.seen);
        const joined = new Uint8Array(data.reduce((n, c) => n + c.length, 0));
        let at = 0;
        for (const c of data) {
          joined.set(c, at);
          at += c.length;
        }
        cursor.pending = append(joined);
        size = Math.min(100, Math.max(1, Math.ceil((remaining * data.length) / Math.max(1, joined.length))));
        setPage(part);
        if (!part.has_more) {
          if (part.finalized && !cursor.pending.length) {
            const tail = cursor.decoder.decode();
            setText((old) => old + tail);
          }
          break;
        }
      }
    } catch (e) {
      if (!controller.signal.aborted) setError((e as Error).message);
    } finally {
      if (!controller.signal.aborted) {
        pending.current = undefined;
        setBusy(false);
      }
    }
  }
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active) void load();
    });
    return () => {
      active = false;
      pending.current?.abort();
    };
  }, []);

  return (
    <section aria-label="Launch log">
      <div className="runner-heading">
        <Typography.Title level={4}>Launch log</Typography.Title>
        <Button
          icon={<DownloadOutlined aria-hidden />}
          href={pulseLogDownload(provider, launch)}
          download={launch + '.log'}
        >
          Download log
        </Button>
      </div>
      {page && (
        <p>
          {page.finalized ? 'Finalized' : 'Live capture'}
          {page.incomplete && ' · Retention gaps reported'}
          {page.estimated_total_size_bytes !== undefined &&
            ` · ${page.estimated_total_size_bytes.toLocaleString()} bytes estimated`}
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {busy && <p role="status">Reading log…</p>}
      {text ? <pre className="runner-log">{text}</pre> : page && <p>No output recorded.</p>}
      {(!page || stream.current.pending.length > 0 || page.has_more || !page.finalized) && (
        <Button disabled={busy} onClick={load}>
          {error
            ? 'Retry log'
            : page && !page.has_more && !stream.current.pending.length
              ? 'Check for new output'
              : 'Show more log'}
        </Button>
      )}
    </section>
  );
}
