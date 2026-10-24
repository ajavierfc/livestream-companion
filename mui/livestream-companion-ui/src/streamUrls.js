export const createVLCIntentURL = streamURL => {
  const url = new URL(streamURL);
  return `intent://${url.host}${url.pathname}${url.search}#Intent;scheme=https;package=org.videolan.vlc;end`;
};
