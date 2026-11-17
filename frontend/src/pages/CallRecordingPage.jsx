import { useSearchParams } from 'react-router-dom';
import AuthAudio from '../components/AuthAudio';

export default function CallRecordingPage() {
  const [params] = useSearchParams();
  const src = params.get('src') || '';
  let valid = src.startsWith('/api/recordings/');
  if (!valid) {
    try { const url = new URL(src); valid = url.protocol === 'https:' && !url.username && !url.password; }
    catch { valid = false; }
  }
  return (
    <main style={{ padding: 28, maxWidth: 760, margin: '0 auto' }}>
      <h1>Call recording</h1>
      {valid ? <>
        <p>Play the recording below. Your account permissions apply to private recordings.</p>
        <AuthAudio key={src} src={src} style={{ width: '100%' }} />
      </> : <p role="alert">This recording link is invalid.</p>}
    </main>
  );
}
