export const createVLCIntentURL = streamURL => {
  const url = new URL(streamURL);
  const scheme = url.protocol.slice(0, -1);
  return `intent://${url.host}${url.pathname}${url.search}#Intent;scheme=${scheme};package=org.videolan.vlc;end`;
};
