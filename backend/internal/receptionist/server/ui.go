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
  .app { width:min(880px,100%); height:min(95vh,1100px); background:var(--panel);
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
  /* Past Conversations panel — sits below the call widget. Each row
     shows a timestamp, a one-line transcript preview, an inline audio
     player, and a delete button. The whole panel collapses if there
     are no recordings. */
  .past { border-top:1px solid #334155; padding:12px 16px;
          max-height:280px; overflow-y:auto; }
  .past-head { display:flex; align-items:center; gap:8px;
               margin-bottom:8px; }
  .past-head h2 { margin:0; font-size:13px; font-weight:600;
                  color:var(--muted); text-transform:uppercase;
                  letter-spacing:.5px; flex:1; }
  .past-head button { padding:6px 10px; font-size:11px;
                      background:#7f1d1d; color:#fee2e2; border:none;
                      border-radius:6px; cursor:pointer; }
  .past-head button:disabled { opacity:.4; cursor:default; }
  .past-empty { font-size:12px; color:var(--muted); padding:8px 0; }
  .rec { background:#0f172a; border-radius:10px; padding:10px 12px;
         margin-bottom:8px; }
  .rec-row { display:flex; align-items:center; gap:8px; flex-wrap:wrap; }
  .rec-ts { font-size:12px; color:var(--muted); flex:1; min-width:120px; }
  .rec audio { height:30px; max-width:220px; }
  .rec-del { padding:5px 9px; font-size:11px; background:#7f1d1d;
             color:#fee2e2; border:none; border-radius:6px;
             cursor:pointer; }
  .rec-confirm { display:flex; gap:6px; align-items:center;
                 font-size:11px; color:var(--muted); }
  .rec-confirm button { padding:5px 9px; font-size:11px; border:none;
                        border-radius:6px; cursor:pointer; }
  .rec-confirm .yes { background:#dc2626; color:#fff; }
  .rec-confirm .no  { background:#475569; color:var(--ink); }
  .rec-preview { font-size:12px; color:var(--ink); margin-top:6px;
                 line-height:1.4; max-height:54px; overflow:hidden;
                 text-overflow:ellipsis; }
  .rec details { margin-top:6px; }
  .rec details summary { font-size:11px; color:var(--muted);
                         cursor:pointer; user-select:none; }
  .rec-line { font-size:12px; padding:4px 0; line-height:1.4; }
  .rec-line.user { color:#93c5fd; }
  .rec-line.assistant { color:#e2e8f0; }
  .rec-line .role { display:inline-block; width:54px; font-weight:600;
                    text-transform:uppercase; font-size:10px;
                    color:var(--muted); }
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
    <button data-text="I'd like to book an appointment with Dr. John tomorrow at 10am">Book Dr. John</button>
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
  <div class="past" id="past">
    <div class="past-head">
      <h2>Past conversations</h2>
      <button id="pastClear" disabled>Delete all</button>
      <div id="pastClearConfirm" class="rec-confirm" style="display:none;">
        <span id="pastClearPrompt">Delete all? This cannot be undone.</span>
        <button class="yes" id="pastClearYes">Confirm</button>
        <button class="no"  id="pastClearNo">Cancel</button>
      </div>
    </div>
    <div id="pastList"></div>
  </div>
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
// pickSystemVoice: choose a SpeechSynthesis voice whose name hints at the
// requested gender. Browsers don't expose a gender field, so we match against
// known male/female system voice names (macOS, Windows, Chrome OS). Used only
// for the speechSynthesis fallback when /tts errors — without this the OS
// picks an arbitrary default that often disagrees with the user's selection.
const SYS_MALE_HINTS=['male','daniel','alex','fred','aaron','tom','david','mark','george','oliver','arthur','rishi','google uk english male','google us english male'];
const SYS_FEMALE_HINTS=['female','samantha','victoria','karen','tessa','moira','fiona','susan','allison','ava','serena','zira','google uk english female','google us english female'];
function pickSystemVoice(gender){
  if(!window.speechSynthesis)return null;
  const voices=window.speechSynthesis.getVoices()||[];
  if(!voices.length)return null;
  const wantMale=String(gender).toLowerCase()==='male';
  const hints=wantMale?SYS_MALE_HINTS:SYS_FEMALE_HINTS;
  const enVoices=voices.filter(v=>/^en[-_]/i.test(v.lang||''));
  const pool=enVoices.length?enVoices:voices;
  // Try gender-hinted match first, then any English voice as last resort.
  for(const h of hints){const m=pool.find(v=>(v.name||'').toLowerCase().includes(h));if(m)return m;}
  return pool[0]||null;
}
// Some browsers populate getVoices() asynchronously — trigger a load so the
// list is ready by the time the fallback path runs.
if(window.speechSynthesis){try{window.speechSynthesis.getVoices();window.speechSynthesis.onvoiceschanged=()=>{};}catch{}}

// ─── Past Conversations: recording machinery ──────────────────────────────
// Mixes the user's mic and the bot's TTS into a single MediaStream and
// records it with MediaRecorder for the duration of a call. On call end
// the blob + transcript get uploaded to /recordings, then re-rendered in
// the panel below the chat. recorderID is per-browser (localStorage) so
// each device sees only its own recordings.
function ensureRecorderID(){
  let id=null;try{id=localStorage.getItem('rec_id');}catch{}
  if(!id||!/^[A-Za-z0-9_-]{1,64}$/.test(id)){
    const a=new Uint8Array(12);crypto.getRandomValues(a);
    id=Array.from(a,b=>b.toString(16).padStart(2,'0')).join('');
    try{localStorage.setItem('rec_id',id);}catch{}
  }
  return id;
}
const RECORDER_ID=ensureRecorderID();
let recCtx=null;          // shared AudioContext for the recording graph
let recMixDest=null;      // MediaStreamAudioDestinationNode receiving mic+bot
let recMicNode=null;      // mic source node connected to recMixDest
let mediaRecorder=null;   // active MediaRecorder for the current call
let recChunks=[];         // collected blob chunks for the current call
let recStartTS=0;         // performance.now() when recording began
let recTranscript=[];     // [{role,text,ts}] collected per turn

// ensureRecCtx lazily creates an AudioContext + MediaStreamDestination
// so the very first speak() call (which happens before any mic gesture
// in chat mode) doesn't break — recording only kicks in once a call
// starts in 'call' mode and ensureMicAccess succeeded.
function ensureRecCtx(){
  if(!recCtx){
    try{recCtx=new (window.AudioContext||window.webkitAudioContext)();}catch{return null;}
  }
  if(!recMixDest){
    try{recMixDest=recCtx.createMediaStreamDestination();}catch{return null;}
  }
  return recCtx;
}

// pipeMicToRecorder: route the existing audioStream (created by
// ensureMicAccess) into the recording mix. Re-runnable; safe to call
// every call start because we disconnect any prior node first.
function pipeMicToRecorder(){
  if(!audioStream)return;
  const ctx=ensureRecCtx();if(!ctx)return;
  if(recMicNode){try{recMicNode.disconnect();}catch{}recMicNode=null;}
  try{
    recMicNode=ctx.createMediaStreamSource(audioStream);
    recMicNode.connect(recMixDest);
  }catch(e){console.warn('mic pipe failed',e);}
}

// tapBotAudio: route an <audio> element through the recording graph so
// the bot's TTS plays through the speakers AND lands in the saved file.
// Must be called BEFORE audioEl.play() — once the element has its own
// playback wired up, the browser won't let WebAudio steal it.
//
// createMediaElementSource detaches default playback, so we MUST connect
// to ctx.destination too or the user hears silence. Browsers also
// require an active (resumed) AudioContext — if it's suspended (no user
// gesture yet), play() succeeds but produces no sound. We resume here.
function tapBotAudio(audioEl){
  const ctx=ensureRecCtx();if(!ctx)return false;
  // Resume the context if it was suspended waiting for a gesture. The
  // mic-permission dialog already provided one, so this almost always
  // succeeds in call mode. Awaited via a noop chain — fire-and-forget
  // is fine because the very next line only needs the *node* graph,
  // and audio output starts asynchronously inside play().
  if(ctx.state==='suspended'){ctx.resume().catch(()=>{});}
  try{
    const src=ctx.createMediaElementSource(audioEl);
    src.connect(ctx.destination);
    if(recMixDest)src.connect(recMixDest);
    return true;
  }catch(e){
    // If createMediaElementSource fails (rare — only happens if the
    // element was already tapped), fall back: play through default
    // output (caller hears it) but recording loses this utterance.
    console.warn('bot audio tap failed',e);return false;
  }
}

function startRecorder(){
  const ctx=ensureRecCtx();if(!ctx||!recMixDest)return;
  if(mediaRecorder&&mediaRecorder.state!=='inactive'){
    try{mediaRecorder.stop();}catch{}
  }
  recChunks=[];recTranscript=[];recStartTS=performance.now();
  // Some browsers don't expose .isTypeSupported (Safari 14-) — try the
  // type and fall back to default if it errors.
  let opts={};
  try{
    if(MediaRecorder.isTypeSupported&&MediaRecorder.isTypeSupported('audio/webm;codecs=opus')){
      opts={mimeType:'audio/webm;codecs=opus'};
    }
  }catch{}
  try{
    mediaRecorder=new MediaRecorder(recMixDest.stream,opts);
  }catch(e){
    console.warn('MediaRecorder unavailable',e);mediaRecorder=null;return;
  }
  mediaRecorder.ondataavailable=ev=>{if(ev.data&&ev.data.size>0)recChunks.push(ev.data);};
  // Capture chunks every 1s so a crash mid-call still yields a partial
  // recording when we eventually flush.
  mediaRecorder.start(1000);
}

async function stopRecorderAndUpload(sessionIdAtEnd){
  if(!mediaRecorder)return;
  const mime=(mediaRecorder.mimeType||'audio/webm').split(';')[0];
  // Wait for the final chunk before upload — onstop fires once the
  // remaining data is flushed via ondataavailable.
  await new Promise(res=>{
    mediaRecorder.onstop=()=>res();
    try{mediaRecorder.stop();}catch{res();}
  });
  const blob=new Blob(recChunks,{type:mime});
  const transcript=recTranscript.slice();
  const duration=Math.round(performance.now()-recStartTS);
  // Skip uploads with no audio at all (e.g. chat-mode session, or call
  // ended before any speech). Avoids littering disk with 0-byte files.
  if(blob.size<2048||transcript.length===0){
    mediaRecorder=null;recChunks=[];recTranscript=[];return;
  }
  const fd=new FormData();
  fd.append('recorder_id',RECORDER_ID);
  if(sessionIdAtEnd)fd.append('session_id',sessionIdAtEnd);
  fd.append('duration_ms',String(duration));
  fd.append('transcript',JSON.stringify(transcript));
  fd.append('audio',blob,'call.webm');
  try{
    const r=await fetch('recordings',{method:'POST',body:fd});
    if(!r.ok){console.warn('upload failed',r.status,await r.text());}
  }catch(e){console.warn('upload error',e);}
  mediaRecorder=null;recChunks=[];recTranscript=[];
  refreshPastList();
}

function recTranscriptPush(role,text){
  if(!text)return;
  recTranscript.push({role,text,ts:Math.floor(Date.now()/1000)});
}

// ─── Past Conversations: list/delete UI ───────────────────────────────────
const pastListEl=document.getElementById('pastList');
const pastClearBtn=document.getElementById('pastClear');
function fmtTS(iso){
  try{const d=new Date(iso);return d.toLocaleString();}catch{return iso;}
}
function fmtDuration(ms){
  if(!ms||ms<0)return '';
  const s=Math.round(ms/1000);
  return s<60?s+'s':Math.floor(s/60)+'m '+(s%60)+'s';
}
async function refreshPastList(){
  let items=[];
  try{
    const r=await fetch('recordings?recorder_id='+encodeURIComponent(RECORDER_ID));
    if(r.ok){const d=await r.json();items=d.recordings||[];}
  }catch(e){console.warn('list failed',e);}
  pastListEl.innerHTML='';
  pastClearBtn.disabled=items.length===0;
  if(items.length===0){
    const e=document.createElement('div');e.className='past-empty';
    e.textContent='No past conversations yet — finished calls will appear here.';
    pastListEl.appendChild(e);return;
  }
  for(const it of items){
    const card=document.createElement('div');card.className='rec';
    const row=document.createElement('div');row.className='rec-row';
    const ts=document.createElement('div');ts.className='rec-ts';
    const dur=fmtDuration(it.duration_ms);
    ts.textContent=fmtTS(it.created_at)+(dur?' • '+dur:'');
    const audio=document.createElement('audio');audio.controls=true;audio.preload='none';
    audio.src='recordings/'+encodeURIComponent(it.id)+'/audio?recorder_id='+encodeURIComponent(RECORDER_ID);
    // Delete button swaps in place to a "Delete? [Confirm] [Cancel]" row,
    // so the user never gets a browser-native popup. confirmEl is held
    // here so the cancel handler can flip the UI back without a refresh.
    const del=document.createElement('button');del.className='rec-del';del.textContent='Delete';
    const confirmEl=document.createElement('div');confirmEl.className='rec-confirm';confirmEl.style.display='none';
    const promptText=document.createElement('span');promptText.textContent='Delete?';
    const yes=document.createElement('button');yes.className='yes';yes.textContent='Confirm';
    const no=document.createElement('button');no.className='no';no.textContent='Cancel';
    confirmEl.appendChild(promptText);confirmEl.appendChild(yes);confirmEl.appendChild(no);
    del.onclick=()=>{del.style.display='none';confirmEl.style.display='flex';};
    no.onclick=()=>{confirmEl.style.display='none';del.style.display='';};
    yes.onclick=async()=>{
      yes.disabled=true;no.disabled=true;promptText.textContent='Deleting…';
      try{
        const r=await fetch('recordings/'+encodeURIComponent(it.id)+'?recorder_id='+encodeURIComponent(RECORDER_ID),{method:'DELETE'});
        if(!r.ok){
          // Inline-revert the prompt with an error message so the user
          // can retry without losing context. No browser alert.
          promptText.textContent='Failed ('+r.status+'). Retry?';
          yes.disabled=false;no.disabled=false;return;
        }
      }catch(e){
        promptText.textContent='Network error. Retry?';
        yes.disabled=false;no.disabled=false;return;
      }
      refreshPastList();
    };
    row.appendChild(ts);row.appendChild(audio);row.appendChild(del);row.appendChild(confirmEl);
    card.appendChild(row);
    // Show the first 1-2 lines as a preview, with the full transcript
    // tucked behind a <details> so the panel stays compact.
    const tr=Array.isArray(it.transcript)?it.transcript:[];
    if(tr.length){
      const preview=document.createElement('div');preview.className='rec-preview';
      preview.textContent=tr.slice(0,2).map(l=>(l.role==='user'?'You: ':'Bot: ')+l.text).join(' · ');
      card.appendChild(preview);
      const det=document.createElement('details');
      const sum=document.createElement('summary');sum.textContent='View full transcript ('+tr.length+' messages)';
      det.appendChild(sum);
      for(const line of tr){
        const ln=document.createElement('div');ln.className='rec-line '+(line.role==='user'?'user':'assistant');
        const role=document.createElement('span');role.className='role';
        role.textContent=line.role==='user'?'You':'Bot';
        ln.appendChild(role);
        ln.appendChild(document.createTextNode(line.text));
        det.appendChild(ln);
      }
      card.appendChild(det);
    }
    pastListEl.appendChild(card);
  }
}
// "Delete all" toggles to an inline Confirm/Cancel pair instead of a
// blocking browser popup. Errors also surface inline by rewriting the
// prompt text — no alert dialogs anywhere in this widget.
const pastClearConfirm=document.getElementById('pastClearConfirm');
const pastClearYes=document.getElementById('pastClearYes');
const pastClearNo=document.getElementById('pastClearNo');
const pastClearPrompt=document.getElementById('pastClearPrompt');
function pastClearReset(){
  pastClearPrompt.textContent='Delete all? This cannot be undone.';
  pastClearYes.disabled=false;pastClearNo.disabled=false;
  pastClearConfirm.style.display='none';pastClearBtn.style.display='';
}
pastClearBtn.addEventListener('click',()=>{
  pastClearBtn.style.display='none';pastClearConfirm.style.display='flex';
});
pastClearNo.addEventListener('click',pastClearReset);
pastClearYes.addEventListener('click',async()=>{
  pastClearYes.disabled=true;pastClearNo.disabled=true;
  pastClearPrompt.textContent='Deleting…';
  try{
    const r=await fetch('recordings?recorder_id='+encodeURIComponent(RECORDER_ID),{method:'DELETE'});
    if(!r.ok){
      pastClearPrompt.textContent='Failed ('+r.status+'). Retry?';
      pastClearYes.disabled=false;pastClearNo.disabled=false;return;
    }
  }catch(e){
    pastClearPrompt.textContent='Network error. Retry?';
    pastClearYes.disabled=false;pastClearNo.disabled=false;return;
  }
  pastClearReset();
  refreshPastList();
});
refreshPastList();
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
  const gender=(genderSel&&genderSel.value)||'female';
  try{
    const voice_id=pickedVoiceId();
    const r=await fetch('tts',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text,gender,voice_id})});
    if(!r.ok){
      // Surface upstream error so the user knows why we fell back to system TTS
      // (otherwise a misconfigured ELEVENLABS key silently produces a wrong-gender voice).
      let detail='';try{const j=await r.clone().json();detail=j.error||'';}catch{try{detail=await r.text();}catch{}}
      notice('TTS error '+r.status+(detail?': '+detail:'')+' — falling back to browser voice.');
      throw new Error('tts '+r.status);
    }
    const blob=await r.blob();
    const url=URL.createObjectURL(blob);
    const a=new Audio(url);currentAudio=a;
    // Tap the bot voice into the recording graph BEFORE play(). In
    // call mode this routes the audio through WebAudio so it lands in
    // both the speakers and the MediaRecorder stream. In chat mode we
    // skip the tap and play directly (no recording is happening).
    let tapped=false;
    if(mode==='call'){tapped=tapBotAudio(a);}
    a.onended=a.onerror=()=>{URL.revokeObjectURL(url);if(currentAudio===a)currentAudio=null;speaking=false;resolve();};
    await a.play();
    // If the WebAudio tap failed in call mode, the browser still plays
    // the audio via the element's default output — recording will miss
    // this utterance but the caller still hears it.
    void tapped;
  }catch(e){
    // Fallback: browser TTS so the page isn't silent if /tts errors.
    // Pick a system voice matching the selected gender so the fallback honors
    // the user's choice instead of using whatever the OS default happens to be.
    if(window.speechSynthesis){
      window.speechSynthesis.cancel();
      const u=new SpeechSynthesisUtterance(text);u.lang='en-US';
      const v=pickSystemVoice(gender);if(v)u.voice=v;
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
  // Voice mode: pipe the mic into the recording graph and start the
  // MediaRecorder so the whole call lands in /recordings on end. Chat
  // mode skips this — there's no audio to record.
  if(mode==='call'&&audioStream){
    pipeMicToRecorder();
    startRecorder();
  }
  const agent_name=pickedPersonaName();
  const r=await fetch('start-call',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({caller_id:'browser-voice-demo',agent_name})});
  const d=await r.json();sessionId=d.session_id;setBadge(d.state,'live');
  addMsg('bot',d.message,{meta:'session '+sessionId.slice(0,8)});
  recTranscriptPush('assistant',d.message);
  input.disabled=false;await speak(d.message);setMic('off');
  setStatus(recog?'Listening… go ahead.':'Type a message and press Enter.');
  autoListen();
}
async function send(text){
  if(!sessionId)return;
  addMsg('user',text);recTranscriptPush('user',text);
  input.value='';setStatus('Thinking…');setMic('off');
  let d;
  try{
    const r=await fetch('process-input',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionId,text})});
    if(!r.ok){addMsg('bot','[error] '+(await r.text()));return;}
    d=await r.json();
  }catch(e){addMsg('bot','[network error] '+e.message);return;}
  setBadge(d.state,d.is_emergency?'warn':'live');
  const meta='intent: '+d.intent+(d.metadata&&Object.keys(d.metadata).length?' • '+JSON.stringify(d.metadata):'');
  addMsg('bot',d.message,{emergency:d.is_emergency,meta});
  recTranscriptPush('assistant',d.message);
  await speak(d.message);
  if(d.state==='ended'){
    input.disabled=true;setMic('off');
    // Flush the recorder so the just-finished call shows up in Past
    // Conversations. Fire-and-forget — refreshPastList runs after upload.
    stopRecorderAndUpload(sessionId);
    // Clear the chat immediately so the next session starts on a clean
    // canvas. Wait for the user to click "New chat" / "New call" — do
    // NOT auto-start a new session.
    sessionId=null;
    log.innerHTML='';
    setBadge('ended');
    const label=mode==='call'?'New call':'New chat';
    setStatus('Click "'+label+'" to start a new conversation.');
    return;
  }
  // Auto-engage mic for the next caller turn (works for both normal
  // and emergency-followup states — caller still needs to speak).
  setStatus(recog?'Listening… go ahead.':'Type a message and press Enter.');
  autoListen();
}
form.addEventListener('submit',e=>{e.preventDefault();const t=input.value.trim();if(t)send(t);});
document.getElementById('restart').addEventListener('click',async()=>{
  const ending=sessionId;
  if(sessionId){try{await fetch('end-call',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionId})});}catch{}}
  // Flush the in-progress recording before starting the next call so it
  // shows up in Past Conversations. Don't await — the new call can
  // proceed in parallel; the panel refreshes when upload completes.
  if(mediaRecorder){stopRecorderAndUpload(ending);}
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
  // Lazy-create + resume the recording AudioContext on this same gesture
  // so subsequent speak() calls can route bot audio through it without
  // hitting Safari's "context suspended; no user gesture" silence.
  const ctx=ensureRecCtx();if(ctx&&ctx.state==='suspended'){try{await ctx.resume();}catch{}}
  document.getElementById('gate').classList.add('hidden');
  startCall();
});

// Mode toggle (chat ↔ call). Run once on load to set the initial visibility,
// and again whenever the dropdown changes. End any in-flight session on
// switch so the new mode starts fresh.
modeSel.addEventListener('change',async()=>{
  const ending=sessionId;
  if(sessionId){try{await fetch('end-call',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:sessionId})});}catch{}}
  if(mediaRecorder){stopRecorderAndUpload(ending);}
  sessionId=null;
  applyMode();
});
applyMode();

// Best-effort save when the tab closes mid-call. We can't await an
// async upload here, but kicking it off lets the request go in flight;
// modern browsers honor in-progress fetches during unload more
// reliably than synchronous XHR.
window.addEventListener('beforeunload',()=>{
  if(mediaRecorder&&mediaRecorder.state==='recording'){
    try{stopRecorderAndUpload(sessionId);}catch{}
  }
});
</script>
</body>
</html>`

func (s *Server) demoUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(demoHTML))
}
