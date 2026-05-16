// sumolite browser client.
//
// Flow:
//   1. POST pairing code on a WebSocket.
//   2. Build a recvonly H.264 transceiver, create offer, send SDP.
//   3. On answer, play the inbound video into <video>.
//   4. Open a "input" data channel and ship mouse/key as 8-byte binary frames.
//
// Latency goals:
//   - Use ondevicechange + playbackRate=1.0 + jitterBufferTarget=0 when
//     the browser supports it (Chrome 120+).
//   - Send input on every input event without rAF batching; the data
//     channel is unordered+unreliable so a dropped move just means the
//     next one wins.

const stage = document.getElementById('stage');
const form = document.getElementById('pair');
const codeEl = document.getElementById('code');
const video = document.getElementById('v');
const hud = document.getElementById('hud');

const EV = { MOVE: 1, BTN: 2, SCROLL: 3, KEY: 4 };

// Pairing is sticky: once a code works, we keep it. Refreshes, reconnects
// after sleep, and re-opens from a bookmarked URL all bypass the form.
// The code is scoped to this host, so different sumolite servers don't
// share state.
const STORAGE_KEY = `sumolite:pair:${location.host}`;
const URL_TOKEN = new URLSearchParams(location.search).get('code') ||
  new URLSearchParams(location.hash.replace(/^#/, '')).get('code');

const showVideo = () => {
  form.classList.add('hidden');
  video.classList.remove('hidden');
  hud.classList.remove('hidden');
};

const showForm = () => {
  form.classList.remove('hidden');
  video.classList.add('hidden');
  hud.classList.add('hidden');
  codeEl.focus();
  codeEl.select();
};

async function tryConnect(token, opts = {}) {
  try {
    await connect(token);
    localStorage.setItem(STORAGE_KEY, token);
    showVideo();
    return true;
  } catch (err) {
    // Bad-token errors should drop the stored code so we don't loop on
    // a stale value. Transient errors keep it.
    if (/bad token|auth/i.test(err.message)) {
      localStorage.removeItem(STORAGE_KEY);
    }
    if (!opts.silent) alert('connect failed: ' + err.message);
    showForm();
    return false;
  }
}

form.addEventListener('submit', (e) => {
  e.preventDefault();
  const token = codeEl.value.trim();
  if (token) tryConnect(token);
});

// Auto-reconnect if we have a code in URL or storage. URL wins so a
// shared link "?code=abc123" always paste-works.
(async () => {
  const saved = URL_TOKEN || localStorage.getItem(STORAGE_KEY);
  if (saved) {
    codeEl.value = saved;
    await tryConnect(saved, { silent: true });
  } else {
    showForm();
  }
})();

async function connect(token) {
  const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`);
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = () => rej(new Error('ws error')); });

  ws.send(JSON.stringify({ type: 'auth', token }));
  const authResp = await nextMsg(ws);
  if (authResp.type !== 'ok') throw new Error(authResp.error || 'auth failed');

  const pc = new RTCPeerConnection({
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
  });

  pc.addTransceiver('video', { direction: 'recvonly' });
  const dc = pc.createDataChannel('input', { ordered: false, maxRetransmits: 0 });
  dc.binaryType = 'arraybuffer';

  pc.ontrack = (ev) => {
    video.srcObject = ev.streams[0];
    // Chromium: target ~0ms jitter buffer for screen content.
    const recv = pc.getReceivers().find(r => r.track && r.track.kind === 'video');
    if (recv && 'jitterBufferTarget' in recv) recv.jitterBufferTarget = 0;
  };

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  ws.send(JSON.stringify({ type: 'offer', sdp: pc.localDescription }));
  const ans = await nextMsg(ws);
  if (ans.type !== 'answer') throw new Error(ans.error || 'no answer');
  await pc.setRemoteDescription(ans.sdp);

  attachInput(video, dc);
  attachHud(pc);
}

function nextMsg(ws) {
  return new Promise((res, rej) => {
    ws.onmessage = (e) => res(JSON.parse(e.data));
    ws.onclose = () => rej(new Error('ws closed'));
  });
}

function attachInput(el, dc) {
  const buf = new ArrayBuffer(8);
  const dv = new DataView(buf);

  const sendMove = (x, y) => {
    if (dc.readyState !== 'open') return;
    dv.setUint8(0, EV.MOVE);
    dv.setInt16(1, x, true);
    dv.setInt16(3, y, true);
    dc.send(buf.slice(0, 5));
  };
  const sendBtn = (b, down) => {
    dv.setUint8(0, EV.BTN);
    dv.setUint8(1, b);
    dv.setUint8(2, down ? 1 : 0);
    dc.send(buf.slice(0, 3));
  };
  const sendScroll = (dx, dy) => {
    dv.setUint8(0, EV.SCROLL);
    dv.setInt16(1, dx, true);
    dv.setInt16(3, dy, true);
    dc.send(buf.slice(0, 5));
  };
  const sendKey = (kc, down) => {
    dv.setUint8(0, EV.KEY);
    dv.setUint16(1, kc, true);
    dv.setUint8(3, down ? 1 : 0);
    dc.send(buf.slice(0, 4));
  };

  const toLocal = (e) => {
    const r = el.getBoundingClientRect();
    const vw = el.videoWidth || r.width;
    const vh = el.videoHeight || r.height;
    const scale = Math.min(r.width / vw, r.height / vh);
    const offX = (r.width - vw * scale) / 2;
    const offY = (r.height - vh * scale) / 2;
    const x = Math.round(((e.clientX - r.left) - offX) / scale);
    const y = Math.round(((e.clientY - r.top) - offY) / scale);
    return [x, y];
  };

  el.addEventListener('mousemove', (e) => { const [x, y] = toLocal(e); sendMove(x, y); });
  el.addEventListener('mousedown', (e) => { sendBtn(e.button, true); e.preventDefault(); });
  el.addEventListener('mouseup',   (e) => { sendBtn(e.button, false); });
  el.addEventListener('wheel',     (e) => { sendScroll(e.deltaX|0, e.deltaY|0); e.preventDefault(); }, { passive: false });
  el.addEventListener('contextmenu', (e) => e.preventDefault());

  window.addEventListener('keydown', (e) => { sendKey(e.keyCode, true); e.preventDefault(); });
  window.addEventListener('keyup',   (e) => { sendKey(e.keyCode, false); e.preventDefault(); });

  // Pointer lock for relative-motion games / desktop apps that hate
  // absolute-position cursors.
  el.addEventListener('dblclick', () => el.requestPointerLock?.());
}

function attachHud(pc) {
  setInterval(async () => {
    const stats = await pc.getStats();
    let fps = 0, kbps = 0, jitter = 0, rtt = 0;
    stats.forEach(r => {
      if (r.type === 'inbound-rtp' && r.kind === 'video') {
        fps = r.framesPerSecond || 0;
        kbps = Math.round((r.bytesReceived * 8) / 1000 / (r.timestamp / 1000));
        jitter = (r.jitter || 0) * 1000;
      }
      if (r.type === 'candidate-pair' && r.state === 'succeeded') {
        rtt = (r.currentRoundTripTime || 0) * 1000;
      }
    });
    hud.textContent = `${fps|0}fps  ${kbps}kbps  jit ${jitter.toFixed(1)}ms  rtt ${rtt.toFixed(1)}ms`;
  }, 1000);
}
