import { createVLCIntentURL } from './streamUrls';

describe('createVLCIntentURL', () => {
  it('preserves HTTPS for VLC', () => {
    expect(createVLCIntentURL('https://iptv.example.test/hls/12.ts?secure=token')).toBe(
      'intent://iptv.example.test/hls/12.ts?secure=token#Intent;scheme=https;package=org.videolan.vlc;end'
    );
  });

  it('preserves HTTP for VLC', () => {
    expect(createVLCIntentURL('http://iptv.example.test/hls/12.ts')).toBe(
      'intent://iptv.example.test/hls/12.ts#Intent;scheme=http;package=org.videolan.vlc;end'
    );
  });
});
