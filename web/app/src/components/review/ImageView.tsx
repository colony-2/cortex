import { useEffect, useState } from 'react';
import { Alert, Button, Space } from 'antd';
import { imageDimensions, type DocumentData } from './document';
export default function ImageView({ data, name }: { data: DocumentData; name: string }) {
  const [image, setImage] = useState<{ url: string; width: number; height: number }>();
  const [error, setError] = useState('');
  const [zoom, setZoom] = useState(0);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    setError(''); setImage(undefined); setZoom(0);
    let url: string | undefined;
    try {
      const [width, height] = imageDimensions(data.bytes, data.mime || '');
      url = URL.createObjectURL(new Blob([data.bytes.slice().buffer], { type: data.mime }));
      setImage({ url, width, height });
    } catch (e) { setError((e as Error).message); }
    return () => { if (url) URL.revokeObjectURL(url); };
  }, [data, retry]);
  if (error) return <Alert type="error" message={error} action={<Button onClick={() => setRetry(v => v + 1)}>Retry image</Button>} />;
  if (!image) return null;
  return <>
    <Space wrap><span>{image.width} × {image.height}</span><Button onClick={() => setZoom(0)}>Fit</Button><Button onClick={() => setZoom(1)}>Actual size</Button><Button onClick={() => setZoom(Math.min(8, (zoom || 1) * 1.25))}>Zoom in</Button><Button onClick={() => setZoom(Math.max(.1, (zoom || 1) / 1.25))}>Zoom out</Button></Space>
    <div className="review-image-scroll" tabIndex={0} role="region" aria-label="Image preview">
      <img src={image.url} alt={name} width={zoom ? image.width * zoom : undefined} style={{ maxWidth: zoom ? 'none' : '100%', height: 'auto' }} onError={() => setError('Could not decode this image. Retry or download the original.')} />
    </div>
  </>;
}
