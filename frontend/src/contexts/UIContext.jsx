import React, { createContext, useCallback, useContext, useState, useRef } from 'react';

// UIContext provides three replacements for the browser-native popups:
//   useToast()   — toast(msg, kind?)  ▸ replaces window.alert(...)
//   useConfirm() — confirm({...})     ▸ replaces window.confirm(...)
//   usePrompt()  — prompt({...})      ▸ replaces window.prompt(...)
//
// Why: the browser-native popups break the dark dashboard's design,
// can't be styled, block the entire UI, and on some platforms surface
// the origin (e.g. "localhost:5173 says") which is alarming for users.
// Inline modals/toasts give the same semantics with consistent styling.

const ToastCtx   = createContext(null);
const ConfirmCtx = createContext(null);
const PromptCtx  = createContext(null);

// useToast returns a function. Call it as toast('done!'), or
// toast('failed', 'error'), or toast({ message, kind, duration }).
export function useToast()   { return useContext(ToastCtx);   }

// useConfirm returns an async function. Resolves to true (OK)
// or false (Cancel). Caller awaits it like:
//   if (!await confirm({ message: 'Delete?' })) return;
export function useConfirm() { return useContext(ConfirmCtx); }

// usePrompt returns an async function. Resolves to the typed string
// (which may be empty) or null when the user cancels.
export function usePrompt()  { return useContext(PromptCtx);  }

// Style block injected once. Kept inline so this provider has zero
// additional file dependencies — drop it into main.jsx and forget.
const STYLE_ID = 'ui-providers-styles';
function ensureStyles() {
  if (typeof document === 'undefined') return;
  if (document.getElementById(STYLE_ID)) return;
  const el = document.createElement('style');
  el.id = STYLE_ID;
  el.textContent = `
.uip-toast-stack{position:fixed;top:20px;right:20px;display:flex;flex-direction:column;gap:8px;z-index:99999;pointer-events:none;max-width:min(420px,calc(100vw - 40px));}
.uip-toast{pointer-events:auto;background:#1e293b;color:#e2e8f0;border-left:4px solid #2563eb;border-radius:8px;padding:12px 16px;box-shadow:0 10px 30px rgba(0,0,0,.4);font-size:14px;line-height:1.4;display:flex;gap:10px;align-items:flex-start;animation:uip-toast-in .18s ease-out;}
.uip-toast.uip-success{border-left-color:#16a34a;}
.uip-toast.uip-error  {border-left-color:#dc2626;}
.uip-toast.uip-warn   {border-left-color:#f59e0b;}
.uip-toast.uip-info   {border-left-color:#2563eb;}
.uip-toast .uip-toast-msg{flex:1;white-space:pre-wrap;word-break:break-word;}
.uip-toast .uip-toast-x{background:transparent;border:none;color:#94a3b8;cursor:pointer;font-size:18px;line-height:1;padding:0 4px;}
.uip-toast .uip-toast-x:hover{color:#e2e8f0;}
@keyframes uip-toast-in{from{transform:translateX(20px);opacity:0;}to{transform:translateX(0);opacity:1;}}
.uip-modal-overlay{position:fixed;inset:0;background:rgba(2,6,23,.7);display:flex;align-items:center;justify-content:center;z-index:99998;animation:uip-fade-in .15s ease-out;}
.uip-modal{background:#1e293b;color:#e2e8f0;border:1px solid #334155;border-radius:12px;padding:22px;width:min(440px,calc(100vw - 40px));box-shadow:0 20px 60px rgba(0,0,0,.5);}
.uip-modal-title{margin:0 0 8px;font-size:16px;font-weight:600;}
.uip-modal-msg{margin:0 0 18px;font-size:14px;line-height:1.5;color:#cbd5e1;white-space:pre-wrap;}
.uip-modal-input{width:100%;padding:10px 12px;border-radius:8px;border:1px solid #334155;background:#0f172a;color:#e2e8f0;font-size:14px;outline:none;margin-bottom:18px;}
.uip-modal-input:focus{box-shadow:0 0 0 2px #2563eb;}
.uip-modal-actions{display:flex;gap:8px;justify-content:flex-end;}
.uip-btn{padding:8px 16px;border:none;border-radius:8px;font-size:13px;font-weight:500;cursor:pointer;}
.uip-btn-primary{background:#2563eb;color:#fff;}
.uip-btn-primary:hover{background:#1d4ed8;}
.uip-btn-danger{background:#dc2626;color:#fff;}
.uip-btn-danger:hover{background:#b91c1c;}
.uip-btn-secondary{background:#475569;color:#e2e8f0;}
.uip-btn-secondary:hover{background:#334155;}
@keyframes uip-fade-in{from{opacity:0;}to{opacity:1;}}
`;
  document.head.appendChild(el);
}

// Pick a kind heuristically from the message when caller didn't pass
// one. Most existing alerts are error reports; keywords like "failed"
// / "error" / "denied" should colour red, "saved"/"sent" green.
function inferKind(msg) {
  const m = String(msg || '').toLowerCase();
  if (/(fail|error|denied|invalid|unable|cannot|couldn'?t|not\s+supported)/.test(m)) return 'error';
  if (/(success|saved|copied|sent|generated|imported|added|started|complete)/.test(m)) return 'success';
  return 'info';
}

export function UIProvider({ children }) {
  ensureStyles();

  // ─── Toasts ──────────────────────────────────────────────────────────
  const [toasts, setToasts] = useState([]);
  const idRef = useRef(0);

  const dismiss = useCallback((id) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  // toast(msg) | toast(msg, 'error') | toast({ message, kind, duration })
  const toast = useCallback((arg, kindArg) => {
    let message, kind, duration;
    if (arg && typeof arg === 'object' && !Array.isArray(arg)) {
      message  = String(arg.message ?? '');
      kind     = arg.kind || inferKind(message);
      duration = arg.duration ?? 4000;
    } else {
      message  = String(arg ?? '');
      kind     = kindArg || inferKind(message);
      duration = 4000;
    }
    if (!message) return;
    const id = ++idRef.current;
    setToasts((prev) => [...prev, { id, message, kind }]);
    if (duration > 0) {
      setTimeout(() => dismiss(id), duration);
    }
    return id;
  }, [dismiss]);

  // ─── Confirm ─────────────────────────────────────────────────────────
  // Single-modal stack — last-asked wins; if a second confirm fires before
  // the first resolves, we queue. Most callers await sequentially so this
  // queue almost never has more than one entry, but it prevents lost
  // resolves when two flows fire in parallel.
  const [confirms, setConfirms] = useState([]); // [{id, opts, resolve}]
  const confirmIdRef = useRef(0);

  const confirm = useCallback((opts = {}) => {
    return new Promise((resolve) => {
      const id = ++confirmIdRef.current;
      const normalized = typeof opts === 'string' ? { message: opts } : opts;
      setConfirms((prev) => [...prev, { id, opts: normalized, resolve }]);
    });
  }, []);

  const resolveConfirm = useCallback((id, value) => {
    setConfirms((prev) => {
      const entry = prev.find((c) => c.id === id);
      if (entry) entry.resolve(value);
      return prev.filter((c) => c.id !== id);
    });
  }, []);

  // ─── Prompt ──────────────────────────────────────────────────────────
  const [prompts, setPrompts] = useState([]); // [{id, opts, resolve}]
  const promptIdRef = useRef(0);

  const prompt = useCallback((opts = {}) => {
    return new Promise((resolve) => {
      const id = ++promptIdRef.current;
      const normalized = typeof opts === 'string' ? { message: opts } : opts;
      setPrompts((prev) => [...prev, { id, opts: normalized, resolve }]);
    });
  }, []);

  const resolvePrompt = useCallback((id, value) => {
    setPrompts((prev) => {
      const entry = prev.find((p) => p.id === id);
      if (entry) entry.resolve(value);
      return prev.filter((p) => p.id !== id);
    });
  }, []);

  // Show only the topmost confirm/prompt to keep the screen calm.
  const activeConfirm = confirms[0] || null;
  const activePrompt  = prompts[0]  || null;

  return (
    <ToastCtx.Provider value={toast}>
      <ConfirmCtx.Provider value={confirm}>
        <PromptCtx.Provider value={prompt}>
          {children}

          {/* Toast stack */}
          <div className="uip-toast-stack" role="region" aria-live="polite">
            {toasts.map((t) => (
              <div key={t.id} className={`uip-toast uip-${t.kind}`} role="status">
                <div className="uip-toast-msg">{t.message}</div>
                <button
                  className="uip-toast-x"
                  onClick={() => dismiss(t.id)}
                  aria-label="Dismiss"
                >×</button>
              </div>
            ))}
          </div>

          {/* Confirm modal — render only the topmost */}
          {activeConfirm && (
            <ConfirmDialog
              key={activeConfirm.id}
              opts={activeConfirm.opts}
              onCancel={() => resolveConfirm(activeConfirm.id, false)}
              onOK={() => resolveConfirm(activeConfirm.id, true)}
            />
          )}

          {/* Prompt modal — render only the topmost */}
          {activePrompt && (
            <PromptDialog
              key={activePrompt.id}
              opts={activePrompt.opts}
              onCancel={() => resolvePrompt(activePrompt.id, null)}
              onSubmit={(val) => resolvePrompt(activePrompt.id, val)}
            />
          )}
        </PromptCtx.Provider>
      </ConfirmCtx.Provider>
    </ToastCtx.Provider>
  );
}

function ConfirmDialog({ opts, onCancel, onOK }) {
  const {
    title = 'Confirm',
    message = 'Are you sure?',
    okText = 'OK',
    cancelText = 'Cancel',
    danger = false,
  } = opts || {};

  // Keyboard: Esc cancels, Enter confirms — same as the browser dialog.
  const onKey = (e) => {
    if (e.key === 'Escape') { e.preventDefault(); onCancel(); }
    if (e.key === 'Enter')  { e.preventDefault(); onOK(); }
  };

  return (
    <div className="uip-modal-overlay" onClick={onCancel}>
      <div
        className="uip-modal"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={onKey}
        tabIndex={-1}
        ref={(el) => el && el.focus()}
        role="dialog"
        aria-modal="true"
      >
        <h3 className="uip-modal-title">{title}</h3>
        <p className="uip-modal-msg">{message}</p>
        <div className="uip-modal-actions">
          <button className="uip-btn uip-btn-secondary" onClick={onCancel}>{cancelText}</button>
          <button
            className={`uip-btn ${danger ? 'uip-btn-danger' : 'uip-btn-primary'}`}
            onClick={onOK}
            autoFocus
          >{okText}</button>
        </div>
      </div>
    </div>
  );
}

function PromptDialog({ opts, onCancel, onSubmit }) {
  const {
    title = 'Input required',
    message = '',
    placeholder = '',
    defaultValue = '',
    okText = 'OK',
    cancelText = 'Cancel',
    type = 'text',
  } = opts || {};

  const [value, setValue] = useState(defaultValue);
  const onKey = (e) => {
    if (e.key === 'Escape') { e.preventDefault(); onCancel(); }
    if (e.key === 'Enter')  { e.preventDefault(); onSubmit(value); }
  };
  return (
    <div className="uip-modal-overlay" onClick={onCancel}>
      <div
        className="uip-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <h3 className="uip-modal-title">{title}</h3>
        {message && <p className="uip-modal-msg">{message}</p>}
        <input
          className="uip-modal-input"
          type={type}
          value={value}
          placeholder={placeholder}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={onKey}
          autoFocus
        />
        <div className="uip-modal-actions">
          <button className="uip-btn uip-btn-secondary" onClick={onCancel}>{cancelText}</button>
          <button className="uip-btn uip-btn-primary" onClick={() => onSubmit(value)}>{okText}</button>
        </div>
      </div>
    </div>
  );
}
