// Node.js example: start a manual outbound call and stream transcripts + audio.
//
// Usage:
//   BASE=https://your-callified-host API_KEY=sk_live_... \
//   node manual_call.js "Akhil" "+91xxxxxxxxxx"
//
// For browsers, swap `ws` for the native WebSocket and `fetch` is already
// available. Everything else stays the same.

const WebSocket = require("ws"); // npm i ws  (browsers: delete this line)

const BASE = process.env.BASE || "http://localhost:8001";
const API_KEY = process.env.API_KEY;
const [, , name, phone] = process.argv;

if (!API_KEY || !name || !phone) {
  console.error("usage: API_KEY=... node manual_call.js <name> <phone>");
  process.exit(1);
}

async function main() {
  // 1. Kick off the dial.
  const res = await fetch(`${BASE}/api/manual-call`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${API_KEY}`,
    },
    body: JSON.stringify({ name, phone, mode: "dial" }),
  });
  if (!res.ok) throw new Error(`manual-call ${res.status}: ${await res.text()}`);
  const { call_sid, monitor_url } = await res.json();
  console.log(`call_sid=${call_sid}  monitor=${monitor_url}`);

  // 2. Open the monitor WebSocket. The server waits up to 30s for the
  //    carrier to connect the media stream before giving up.
  const wsUrl = BASE.replace(/^http/, "ws") + monitor_url;
  const ws = new WebSocket(wsUrl);

  ws.on("open", () => console.log("monitor connected — waiting for events"));
  ws.on("close", () => console.log("monitor closed"));
  ws.on("error", (e) => console.error("ws error:", e.message));

  ws.on("message", (raw) => {
    const evt = JSON.parse(raw);
    switch (evt.type) {
      case "transcript":
        console.log(`[${evt.role}] ${evt.text}`);
        break;
      case "audio":
        // evt.payload is base64. evt.format is "ulaw_8k" (Exotel) or "pcm16_8k".
        // In a browser you can feed this into a Web Audio API AudioBufferSource;
        // in Node you'd pipe to a file or speaker library. Here we just count.
        process.stdout.write(".");
        break;
      default:
        if (evt.error) console.error("monitor error:", evt.error);
    }
  });
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
