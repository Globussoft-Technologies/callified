package server

import "net/http"

// demoHTML is a single-page voice/chat client. STT via Web Speech API,
// TTS via SpeechSynthesis. Works offline against the REST endpoints.
const demoHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>AI Receptionist — Voice Demo</title>
<style>
  :root { --bg:#0f172a; --panel:#1e293b; --ink:#e2e8f0; --me:#2563eb;
          --bot:#334155; --warn:#dc2626; --ok:#16a34a; --muted:#94a3b8; }
  * { box-sizing: border-box; }
  body { margin:0; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;
         background:var(--bg); color:var(--ink); min-height:100vh; display:flex;
         align-items:center; justify-content:center; padding:16px; }
  .app { width:min(760px,100%); height:min(90vh,860px); background:var(--panel);
         border-radius:14px; display:flex; flex-direction:column; overflow:hidden;
         box-shadow:0 20px 50px rgba(0,0,0,.4); }
  header { padding:14px 18px; border-bottom:1px solid #334155;
           display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
  header h1 { margin:0; font-size:16px; font-weight:600; }
  .pill { font-size:11px; padding:3px 8px; background:#334155;
          border-radius:999px; color:var(--muted); }
  .pill.live { background:var(--ok); color:#fff; }
  .pill.warn { background:var(--warn); color:#fff; }
  .controls { display:flex; gap:6px; margin-left:auto; align-items:center; }
  .controls select, .controls button {
    padding:6px 10px; font-size:12px; background:#475569;
    color:var(--ink); border:none; border-radius:8px; cursor:pointer; }
  .controls select { background:#0f172a; color:var(--ink); }
  #log { flex:1; overflow-y:auto; padding:18px; display:flex; flex-direction:column; gap:10px; }
  .msg { max-width:80%; padding:10px 14px; border-radius:14px; line-height:1.4;
         white-space:pre-wrap; word-wrap:break-word; }
  .msg.user { align-self:flex-end; background:var(--me); color:#fff;
              border-bottom-right-radius:4px; }
  .msg.bot { align-self:flex-start; background:var(--bot);
             border-bottom-left-radius:4px; }
  .msg.emergency { background:var(--warn); color:#fff; font-weight:500; }
  .msg.partial { opacity:.6; font-style:italic; }
  .meta { font-size:11px; color:var(--muted); margin-top:4px; }
  .callbar { display:none; }
  .mic { width:56px; height:56px; border-radius:50%; border:none;
         background:var(--me); color:#fff; font-size:22px; cursor:pointer;
         display:flex; align-items:center; justify-content:center;
         flex-shrink:0; transition:transform .1s; }
  .mic:active { transform:scale(.95); }
  .mic.listening { background:var(--ok); animation:pulse 1.2s infinite; }
  .mic.speaking { background:#a855f7; }
  .mic.off { background:#475569; }
  @keyframes pulse { 0%,100% { box-shadow:0 0 0 0 rgba(22,163,74,.6); }
                     50% { box-shadow:0 0 0 12px rgba(22,163,74,0); } }
  .level { width:80px; height:6px; background:#334155; border-radius:3px;
           overflow:hidden; margin-left:8px; display:none; }
  .level.on { display:inline-block; }
  .level-bar { height:100%; width:0%; background:var(--ok);
               transition:width .1s linear; }
  .status { flex:1; font-size:13px; color:var(--muted); }
  form { display:flex; gap:8px; padding:0 14px 14px; }
  input[type=text] { flex:1; padding:10px 14px; border-radius:10px; border:none;
                     background:#0f172a; color:var(--ink); font-size:14px; outline:none; }
  input[type=text]:focus { box-shadow:0 0 0 2px var(--me); }
  form button { padding:10px 18px; border:none; border-radius:10px;
                background:var(--me); color:#fff; font-weight:500; cursor:pointer; }
  .suggest { display:none; }
  .suggest button { font-size:12px; padding:6px 10px; background:#475569;
                    color:var(--ink); border:none; border-radius:8px; cursor:pointer; }
  .notice { padding:10px 14px; background:#7c2d12; color:#fed7aa;
            font-size:12px; display:none; }
  .gate { position:fixed; inset:0; background:rgba(15,23,42,.92);
          display:flex; align-items:center; justify-content:center;
          flex-direction:column; gap:18px; z-index:10; }
  .gate.hidden { display:none; }
  .gate h2 { margin:0; font-size:22px; font-weight:600; }
  .gate p { margin:0; max-width:400px; text-align:center; color:var(--muted); }
  .gate button { padding:14px 32px; border:none; border-radius:12px;
                 background:var(--ok); color:#fff; font-size:16px;
                 font-weight:600; cursor:pointer; }
</style>
</head>
<body>
<div class="gate" id="gate">
  <h2>📞 AI Receptionist</h2>
  <p>Tap below to start a hands-free voice call. Your browser will ask for microphone access — allow it once and the conversation runs without further clicks.</p>
  <button id="gateStart">Tap to call</button>
</div>
<div class="app">
  <header>
    <h1>AI Receptionist</h1>
    <span class="pill" id="state">connecting…</span>
    <div class="controls">
      <label class="pill" style="background:#0f172a;">Mode</label>
      <select id="modeSel">
        <option value="chat">Chat</option>
        <option value="call">Call</option>
      </select>
      <label class="pill" style="background:#0f172a;">Voice</label>
      <select id="voiceGender">
        <option value="female">Female</option>
        <option value="male">Male</option>
      </select>
      <select id="voicePick" title="Specific voice" style="display:none;"></select>
      <button id="restart">New chat</button>
    </div>
  </header>
  <div id="notice" class="notice"></div>
  <div id="log"></div>
  <div class="suggest">
    <button data-text="Hi, my name is Jane Smith">My name is Jane</button>
    <button data-text="I'd like to book an appointment with Dr. Patel tomorrow at 10am">Book Dr. Patel</button>
    <button data-text="What are your hours?">Hours</button>
    <button data-text="Where are you located?">Location</button>
    <button data-text="I have severe chest pain">Emergency test</button>
    <button data-text="No thanks, goodbye">Goodbye</button>
  </div>
  <div class="callbar">
    <button id="mic" class="mic off" title="Hold to talk / click to toggle">🎙</button>
    <div class="status" id="status">Click the mic to start talking — or type below.</div>
    <div class="level" id="level"><div class="level-bar" id="levelBar"></div></div>
  </div>
  <form id="form">
    <input id="text" type="text" autocomplete="off" placeholder="Type a message and press Enter…">
    <button type="submit">Send</button>
  </form>
</div>
<script>
const log=document.getElementById('log'),stateBadge=document.getElementById('state');
const form=document.getElementById('form'),input=document.getElementById('text');
const micBtn=document.getElementById('mic'),statusEl=document.getElementById('status');
const noticeEl=document.getElementById('notice'),genderSel=document.getElementById('voiceGender');
const voiceSel=document.getElementById('voicePick');
const levelEl=document.getElementById('level'),levelBar=document.getElementById('levelBar');
const modeSel=document.getElementById('modeSel'),restartBtn=document.getElementById('restart');
const gateEl=document.getElementById('gate');
let sessionId=null,listening=false,speaking=false,stopping=false;
// Mode: 'chat' (text only — no mic, no TTS) | 'call' (voice — TTS speaks, mic listens).
// Initial value mirrors the default <option> so applyMode() runs the right setup.
let mode='chat';
function applyMode(){
  mode=modeSel.value;
  if(mode==='call'){
    form.style.display='none';
    restartBtn.textContent='New call';
    // Show the gate so the user grants mic permission via a user gesture
    // (browsers require it). Only show it if we haven't started a call yet.
    if(!sessionId){gateEl.classList.remove('hidden');}
  }else{
    form.style.display='';
    restartBtn.textContent='New chat';
    gateEl.classList.add('hidden');
    // Auto-start the chat session on first entry — no mic gesture needed.
    if(!sessionId)startCall();
  }
}
function setStatus(t){statusEl.textContent=t;}
function setBadge(t,k=''){stateBadge.textContent=t;stateBadge.className='pill '+k;}
function setMic(m){micBtn.classList.remove('off','listening','speaking');micBtn.classList.add(m);}
function notice(m){noticeEl.textContent=m;noticeEl.style.display=m?'block':'none';}
function addMsg(role,text,opts={}){
  const d=document.createElement('div');
  d.className='msg '+role+(opts.emergency?' emergency':'')+(opts.partial?' partial':'');
  d.textContent=text;
  if(opts.meta){const m=document.createElement('div');m.className='meta';m.textContent=opts.meta;d.appendChild(m);}
  log.appendChild(d);log.scrollTop=log.scrollHeight;return d;
}
// Curated ElevenLabs voice catalog — premade voices that work on the free tier
// of this account (probed via /v1/text-to-speech/{voice_id} returning 200).
// "Library voices" not in the account return 402 paid_plan_required, so we
// stick to the premade ones the account already has access to.
const ELEVEN_VOICES={
  female:[
    {id:'EXAVITQu4vr4xnSDxMaL',name:'Sarah — Mature, Reassuring'},
    {id:'cgSgspJ2msm6clMCkdW9',name:'Jessica — Playful, Bright, Warm'},
    {id:'hpp4J3VqNfWAUOO0d1Us',name:'Bella — Professional, Bright'},
    {id:'pFZP5JQG7iQjIQuC4Bku',name:'Lily — Velvety Actress'},
    {id:'Xb7hH8MSUJpSbSDYk0k2',name:'Alice — Clear, Engaging Educator'},
    {id:'XrExE9yKIg1WjnnlVkGX',name:'Matilda — Knowledgable, Professional'},
    {id:'FGY2WhTYpPnrIDTdsKH5',name:'Laura — Enthusiast, Quirky'},
  ],
  male:[
    {id:'JBFqnCBsd6RMkjVDRZzb',name:'George — Warm, Captivating Storyteller'},
    {id:'pNInz6obpgDQGcFmaJgB',name:'Adam — Dominant, Firm'},
    {id:'onwK4e9ZLuTAKqWW03F9',name:'Daniel — Steady Broadcaster'},
    {id:'nPczCjzI2devNBz1zQrb',name:'Brian — Deep, Resonant, Comforting'},
    {id:'cjVigY5qzO86Huf0OWal',name:'Eric — Smooth, Trustworthy'},
    {id:'IKne3meq5aSn9XLyUdCD',name:'Charlie — Deep, Confident, Energetic'},
    {id:'CwhRBWXzGAHq8TQ4Fs17',name:'Roger — Laid-Back, Casual'},
    {id:'pqHfZKP75CvOlQylNhV4',name:'Bill — Wise, Mature, Balanced'},
    {id:'TX3LPaxmHKxFdv7VOQHJ',name:'Liam — Energetic, Social Media Creator'},
    {id:'bIHbv24MWmeRgasZH58o',name:'Will — Relaxed Optimist'},
    {id:'iP95p4xoKVk53GoZ742B',name:'Chris — Charming, Down-to-Earth'},
    {id:'N2lVS1w4EtoT3dr4eOWO',name:'Callum — Husky Trickster'},
    {id:'SOYHLrjzK2X1ezoPC6cr',name:'Harry — Fierce Warrior'},
  ],
};
function rebuildVoiceList(){
  const list=ELEVEN_VOICES[genderSel.value]||ELEVEN_VOICES.female;
  voiceSel.innerHTML='';
  list.forEach(v=>{const o=document.createElement('option');o.value=v.id;o.textContent=v.name;voiceSel.appendChild(o);});
}
voiceSel.style.display='';            // show the picker (was display:none)
voiceSel.style.maxWidth='200px';      // keep it tidy in the header bar
rebuildVoiceList();
genderSel.addEventListener('change',rebuildVoiceList);
function pickedVoiceId(){return voiceSel.value||'';}
// speak: server-side ElevenLabs TTS. Posts the text to /tts, gets back an
// MP3 stream, plays it via HTMLAudioElement. Falls back to browser
// speechSynthesis if /tts fails (so a misconfigured key doesn't make the
// page silent — operators still get something audible).
let currentAudio=null;
function speak(text){return new Promise(async resolve=>{
  // Chat mode is text-only — skip TTS playback entirely.
  if(mode==='chat'){return resolve();}
  if(currentAudio){try{currentAudio.pause();}catch{}currentAudio=null;}
  speaking=true;setMic('speaking');setStatus('Speaking…');
  try{
    const gender=(genderSel&&genderSel.value)||'female';
    const voice_id=pickedVoiceId();
    const r=await fetch('tts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text,gender,voice_id})});
    if(!r.ok)throw new Error('tts '+r.status);
    const blob=await r.blob();
    const url=URL.createObjectURL(blob);
    const a=new Audio(url);currentAudio=a;
    a.onended=a.onerror=()=>{URL.revokeObjectURL(url);if(currentAudio===a)currentAudio=null;speaking=false;resolve();};
    await a.play();
  }catch(e){
    // Fallback: browser TTS so the page isn't silent if /tts errors.
    if(window.speechSynthesis){
      window.speechSynthesis.cancel();
      const u=new SpeechSynthesisUtterance(text);u.lang='en-US';
      u.onend=u.onerror=()=>{speaking=false;resolve();};
      window.speechSynthesis.speak(u);
    }else{speaking=false;resolve();}
  }
});}
// Speech recognition with continuous listening + silence-debounce.
// continuous=true keeps the mic open across pauses inside a sentence.
// We accumulate final segments into utteranceBuffer and only send the
// turn after SILENCE_MS of no new speech, so callers can pause to think
// without having their utterance cut off.
const SR=window.SpeechRecognition||window.webkitSpeechRecognition;
const SILENCE_MS=1500;  // adjust if callers feel rushed or sluggish
const NO_AUDIO_WARN_MS=6000;  // warn if no audio detected this long
let recog=null,partialNode=null,utteranceBuffer='',silenceTimer=null;
let audioStream=null,audioCtx=null,analyser=null,levelTimer=null,noAudioTimer=null;
let sawAudio=false;

// Web Audio level meter: gives the caller visible feedback that the mic
// is actually receiving sound. Without this, a silent recognizer looks
// identical to a working one that just hasn't been spoken to.
async function ensureMicAccess(){
  if(audioStream)return true;
  try{
    audioStream=await navigator.mediaDevices.getUserMedia({audio:true});
    audioCtx=new (window.AudioContext||window.webkitAudioContext)();
    const src=audioCtx.createMediaStreamSource(audioStream);
    analyser=audioCtx.createAnalyser();
    analyser.fftSize=256;
    src.connect(analyser);
    return true;
  }catch(err){
    notice('Microphone access denied or unavailable: '+err.message+'. Click the mic icon in the address bar to allow it, or type below.');
    return false;
  }
}
function startLevelMeter(){
  if(!analyser)return;
  levelEl.classList.add('on');
  const data=new Uint8Array(analyser.frequencyBinCount);
  sawAudio=false;
  function tick(){
    if(!listening){levelEl.classList.remove('on');levelBar.style.width='0%';return;}
    analyser.getByteFrequencyData(data);
    let sum=0;for(let i=0;i<data.length;i++)sum+=data[i];
    const avg=sum/data.length;          // 0-255
    const pct=Math.min(100,Math.round(avg*1.5));
    levelBar.style.width=pct+'%';
    if(avg>8)sawAudio=true;
    levelTimer=requestAnimationFrame(tick);
  }
  tick();
}
function stopLevelMeter(){
  if(levelTimer){cancelAnimationFrame(levelTimer);levelTimer=null;}
  levelEl.classList.remove('on');
  levelBar.style.width='0%';
}
function startNoAudioWatchdog(){
  if(noAudioTimer)clearTimeout(noAudioTimer);
  noAudioTimer=setTimeout(()=>{
    if(listening&&!sawAudio){
      notice('I am not hearing any audio. Make sure the mic icon next to the URL bar is enabled and no other app is using your microphone.');
    }
  },NO_AUDIO_WARN_MS);
}
function clearSilenceTimer(){if(silenceTimer){clearTimeout(silenceTimer);silenceTimer=null;}}
function flushUtterance(){
  clearSilenceTimer();
  stopLevelMeter();
  if(noAudioTimer){clearTimeout(noAudioTimer);noAudioTimer=null;}
  const text=utteranceBuffer.trim();
  utteranceBuffer='';
  if(partialNode){partialNode.remove();partialNode=null;}
  if(!text)return;
  // Stop the recognizer so onend fires; the auto-listen logic restarts
  // it after the bot replies.
  stopping=true;
  try{recog.stop();}catch{}
  send(text);
}
if(!SR){notice('Your browser does not support the Web Speech API. Use Chrome/Edge/Safari for voice; typing still works.');micBtn.disabled=true;}
else{
  recog=new SR();
  recog.lang='en-US';
  recog.interimResults=true;
  recog.continuous=true;
  recog.onresult=(e)=>{
    let interim='',justFinal='';
    for(let i=e.resultIndex;i<e.results.length;i++){
      const r=e.results[i];if(r.isFinal)justFinal+=r[0].transcript;else interim+=r[0].transcript;
    }
    if(justFinal)utteranceBuffer+=justFinal;
    const display=(utteranceBuffer+' '+interim).trim();
    if(display){
      if(!partialNode)partialNode=addMsg('user',display,{partial:true});
      else partialNode.textContent=display;
    }
    // Reset the silence countdown on any new sound (interim or final).
    clearSilenceTimer();
    silenceTimer=setTimeout(flushUtterance,SILENCE_MS);
  };
  recog.onerror=(e)=>{
    if(e.error==='not-allowed')notice('Microphone permission denied.');
    else if(e.error!=='no-speech'&&e.error!=='aborted')setStatus('Mic error: '+e.error);
  };
  recog.onend=()=>{
    listening=false;
    clearSilenceTimer();
    if(!stopping&&sessionId&&!speaking){
      // Browser ended the stream on us mid-listen (Chrome does this
      // periodically). Restart so the caller can keep talking.
      try{recog.start();listening=true;return;}catch{}
    }
    if(stopping){setMic('off');setStatus('');stopping=false;}
    else{setMic('off');setStatus('');}
  };
}
async function startListening(){
  if(!recog||listening||speaking)return;
  if(!await ensureMicAccess())return;
  utteranceBuffer='';
  try{
    recog.start();
    listening=true;
    setMic('listening');
    setStatus('Listening… take your time.');
    startLevelMeter();
    startNoAudioWatchdog();
  }catch(e){
    // recog.start() throws if already started — ignore that case;
    // surface other errors so the user knows something is wrong.
    if(!String(e).includes('already started'))setStatus('Mic error: '+e.message);
  }
}
function stopListening(){
  if(!recog||!listening)return;
  stopping=true;
  stopLevelMeter();
  if(noAudioTimer){clearTimeout(noAudioTimer);noAudioTimer=null;}
  // If there's already buffered speech, send it before stopping.
  if(utteranceBuffer.trim()){flushUtterance();return;}
  try{recog.stop();}catch{}
}
micBtn.addEventListener('click',()=>{if(listening)stopListening();else startListening();});
// Hands-free turn-taking: after every assistant utterance, automatically
// open the mic so the caller can reply without clicking. Skip auto-listen
// only at terminal states (call ended) or when the browser has no SR.
function autoListen(){
  if(mode==='chat')return; // text-only mode never opens the mic
  if(!recog)return;
  // Tiny delay lets system audio buffers clear so TTS doesn't bleed
  // into the recognizer.
  setTimeout(()=>startListening(),200);
}
// pickedPersonaName: extract just the first name (e.g. "Sarah" from
// "Sarah — Mature, Reassuring") so the greeting reads naturally —
// "This is Sarah." rather than "This is Sarah — Mature, Reassuring."
function pickedPersonaName(){
  const opt=voiceSel&&voiceSel.options[voiceSel.selectedIndex];
  if(!opt)return '';
  return (opt.textContent||'').split(/[—\-(]/)[0].trim();
}
async function startCall(){
  log.innerHTML='';setBadge('connecting…');setStatus('Starting call…');
  const agent_name=pickedPersonaName();
  const r=await fetch('start-call',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({caller_id:'browser-voice-demo',agent_name})});
  const d=await r.json();sessionId=d.session_id;setBadge(d.state,'live');
  addMsg('bot',d.message,{meta:'session '+sessionId.slice(0,8)});
  input.disabled=false;await speak(d.message);setMic('off');
  setStatus(recog?'Listening… go ahead.':'Type a message and press Enter.');
  autoListen();
}
async function send(text){
  if(!sessionId)return;
  addMsg('user',text);input.value='';setStatus('Thinking…');setMic('off');
  let d;
  try{
    const r=await fetch('process-input',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionId,text})});
    if(!r.ok){addMsg('bot','[error] '+(await r.text()));return;}
    d=await r.json();
  }catch(e){addMsg('bot','[network error] '+e.message);return;}
  setBadge(d.state,d.is_emergency?'warn':'live');
  const meta='intent: '+d.intent+(d.metadata&&Object.keys(d.metadata).length?' • '+JSON.stringify(d.metadata):'');
  addMsg('bot',d.message,{emergency:d.is_emergency,meta});
  await speak(d.message);
  if(d.state==='ended'){input.disabled=true;setMic('off');setStatus('Call ended. Click "New call".');return;}
  // Auto-engage mic for the next caller turn (works for both normal
  // and emergency-followup states — caller still needs to speak).
  setStatus(recog?'Listening… go ahead.':'Type a message and press Enter.');
  autoListen();
}
form.addEventListener('submit',e=>{e.preventDefault();const t=input.value.trim();if(t)send(t);});
document.getElementById('restart').addEventListener('click',async()=>{
  if(sessionId){try{await fetch('end-call',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionId})});}catch{}}
  sessionId=null;startCall();
});
document.querySelectorAll('.suggest button').forEach(b=>b.addEventListener('click',()=>send(b.dataset.text)));

// One-time gate: a single user gesture is required by every modern
// browser to enable both microphone capture and audio playback. After
// the caller taps once, the entire call (greeting → speak → listen →
// reply → listen → …) runs hands-free.
document.getElementById('gateStart').addEventListener('click',async()=>{
  const ok=await ensureMicAccess();
  if(!ok)return;
  document.getElementById('gate').classList.add('hidden');
  startCall();
});

// Mode toggle (chat ↔ call). Run once on load to set the initial visibility,
// and again whenever the dropdown changes. End any in-flight session on
// switch so the new mode starts fresh.
modeSel.addEventListener('change',async()=>{
  if(sessionId){try{await fetch('end-call',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionId})});}catch{}}
  sessionId=null;
  applyMode();
});
applyMode();
</script>
</body>
</html>`

func (s *Server) demoUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(demoHTML))
}
