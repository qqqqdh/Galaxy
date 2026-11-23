'use strict';

const host = document.getElementById('terminal');
const shellStatus = document.getElementById('status');
const restart = document.getElementById('restart');
const progress = document.getElementById('progress');
const ubuntu = document.getElementById('ubuntu');
const browserButton = document.getElementById('browser');
const back = document.getElementById('back');
const menu = document.getElementById('clipboard-menu');
const clipboard = document.getElementById('clipboard');
const copy = document.getElementById('copy');
const paste = document.getElementById('paste');
const control = document.getElementById('control');
const notification = document.getElementById('notice');
const cursorKeys = { ArrowUp: 'A', ArrowDown: 'B', ArrowRight: 'C', ArrowLeft: 'D' };

function openTerminalLink(event, url) {
  event.preventDefault();
  Android.openLink(url);
}

const term = new Terminal({
  allowProposedApi: true,
  cursorBlink: true,
  fontFamily: 'monospace, "Noto Sans CJK KR", sans-serif',
  fontSize: 15,
  lineHeight: 1.15,
  scrollback: 5000,
  screenReaderMode: true,
  linkHandler: { activate: openTerminalLink },
  theme: {
    background: '#101319', foreground: '#edf1f8', cursor: '#9eddd1',
    selectionBackground: '#374f69', black: '#212631', brightBlack: '#8590a2',
    red: '#ff8b90', green: '#9eddd1', yellow: '#f0d096', blue: '#90baf8',
    magenta: '#c5a7ef', cyan: '#87d7e5', white: '#edf1f8'
  }
});
const fit = new FitAddon.FitAddon();
term.loadAddon(fit);
term.loadAddon(new WebLinksAddon.WebLinksAddon(openTerminalLink));
term.open(host);
term.textarea.setAttribute('aria-label', '터미널 입력');
term.textarea.setAttribute('autocorrect', 'off');
term.textarea.setAttribute('autocapitalize', 'off');
const draftView = document.createElement('span');
draftView.className = 'ide-draft';
draftView.hidden = true;
const screen = host.querySelector('.xterm-screen');
screen.appendChild(draftView);
draftView.style.fontFamily = term.options.fontFamily;
draftView.style.fontSize = term.options.fontSize + 'px';

let running = false;
let atPrompt = false;
let liveEditor = false;
let nativeTerminal = false;
let nativeDraft = '';
let draftAnchor;
let inGalaxyIDE = false;
let liveRange = { text: '' };
let composing;
let settling;
let editorCommit;
let imeInputPending = false;
let noticeTimer;

if (Android.isUbuntuSession()) {
  document.querySelector('h1').textContent = 'Ubuntu 24.04';
  back.hidden = false;
  back.addEventListener('click', () => Android.closeUbuntu());
} else if (Android.supportsUbuntu()) {
  ubuntu.hidden = false;
  ubuntu.addEventListener('click', () => Android.openUbuntu());
}
browserButton.addEventListener('click', () => Android.openBrowser());

function shellProgress(message) {
  progress.textContent = message;
  progress.hidden = false;
}
function notice(message) {
  notification.textContent = message;
  notification.hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => { notification.hidden = true; }, 2200);
}

function receiveOutput(base64) {
  term.write(Uint8Array.from(atob(base64), character => character.charCodeAt(0)), () => Android.outputConsumed());
}

function shellStarted() {
  progress.hidden = true;
  running = true;
  term.options.disableStdin = false;
  shellStatus.textContent = '실행 중';
  shellStatus.dataset.running = 'true';
  updateInputMode();
}

function shellEnded(message) {
  restart.textContent = running ? '새 셸' : '다시 시도';
  progress.hidden = true;
  running = false;
  updateInputMode();
  atPrompt = false;
  composing = settling = null;
  editorCommit = null;
  draftView.hidden = true;
  imeInputPending = false;
  term.options.disableStdin = true;
  control.setAttribute('aria-pressed', 'false');
  shellStatus.textContent = '종료';
  shellStatus.dataset.running = 'false';
  term.write(`\r\n[${message}]\r\n`);
  restart.hidden = false;
}

function openShell() {
  restart.hidden = true;
  shellStatus.textContent = '여는 중';
  fit.fit();
  Android.start(term.cols, term.rows);
}

term.onResize(({ cols, rows }) => Android.resize(cols, rows));
// Screen reader mode leaves navigation defaults active, which can move WebView focus.
term.onKey(({ domEvent }) => {
  if (domEvent.key === 'Tab' || domEvent.key.startsWith('Arrow')) domEvent.preventDefault();
  if (liveEditor && (domEvent.key === 'Enter' || domEvent.key === 'Backspace')) domEvent.preventDefault();
});
new ResizeObserver(() => fit.fit()).observe(host);
function sendInput(data) {
  const accepted = Android.send(data);
  if (accepted && !data.startsWith('\x1b[200~') && /[\r\n]/.test(data)) {
    atPrompt = false;
    updateInputMode();
  }
  return accepted;
}

term.onData(data => {
  if (!running) return;
  if (control.getAttribute('aria-pressed') === 'true') {
    control.setAttribute('aria-pressed', 'false');
    if (/^[a-z@[\\\]^_?]$/i.test(data)) {
      data = data === '?' ? '\x7f' : String.fromCharCode(data.toUpperCase().charCodeAt(0) & 31);
    }
  }
  sendInput(data);
});

term.parser.registerOscHandler(133, message => {
  if (message !== 'B') return false;
  atPrompt = true;
  updateInputMode();
  return true;
});

function replaceDraft(composition, text) {
  const before = Array.from(composition.text);
  const after = Array.from(text);
  let shared = 0;
  while (shared < before.length && shared < after.length && before[shared] === after[shared]) shared++;
  const changes = '\x7f'.repeat(before.length - shared) + after.slice(shared).join('');
  if (!changes || sendInput(changes)) composition.text = text;
}

function inEditorScreen() {
  return running && term.buffer.active.type === 'alternate';
}
function resetLiveRange() {
  liveRange = { text: '' };
  term.textarea.value = '';
}
function updateInputMode() {
  const mode = !running ? 0 : liveEditor ? 2 : inGalaxyIDE ? 1 : (!atPrompt || inEditorScreen()) ? 3 : 0;
  if (mode === 3 && !nativeTerminal) term.focus();
  nativeTerminal = mode === 3;
  Android.editorMode?.(mode);
}
function setEditorTitle(title) {
  const anchor = inEditorScreen() && /^Galaxy IDE · 터미널 (\d+),(\d+)$/.exec(title);
  draftAnchor = anchor ? { x: Number(anchor[1]), y: Number(anchor[2]) } : null;
  inGalaxyIDE = inEditorScreen() && (title === 'Galaxy IDE' || title === 'Galaxy IDE · 편집');
  const enabled = inGalaxyIDE && title === 'Galaxy IDE · 편집';
  if (liveEditor === enabled) { updateInputMode(); positionDraft(); return; }
  composing = settling = editorCommit = null;
  imeInputPending = false;
  draftView.hidden = true;
  liveEditor = enabled;
  resetLiveRange();
  updateInputMode();
}
term.onTitleChange(setEditorTitle);
term.buffer.onBufferChange(() => setEditorTitle(''));

function nativeEditorEdit(erased, inserted) {
  if ((!liveEditor && !nativeTerminal) || erased === inserted) return;
  const before = Array.from(erased);
  const after = Array.from(inserted);
  let shared = 0;
  while (shared < before.length && shared < after.length && before[shared] === after[shared]) shared++;
  const text = after.slice(shared).join('').replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '').replace(/\n/g, '\r');
  term.input('\x7f'.repeat(before.length - shared) + text);
}

function nativeTerminalDraft(text) {
  nativeDraft = nativeTerminal ? text : '';
  draftView.textContent = nativeDraft;
  draftView.hidden = !nativeDraft;
  positionDraft();
}

function editorEscape() {
  if (!menu.hidden) { closeMenu(); term.focus(); return; }
  if (!running) return;
  composing = settling = editorCommit = null;
  draftView.hidden = true;
  term.focus();
  term.input('\x1b');
}

function syncLiveRange() {
  // Samsung can start its next composition by reselecting already committed text.
  // Mirror the actual editable range, including those replacements, across language switches.
  const value = term.textarea.value;
  if (value !== liveRange.text && !/[\x00-\x1f\x7f]/.test(value)) replaceDraft(liveRange, value);
}

function positionDraft() {
  if (!composing?.preview && !nativeDraft) return;
  const cursor = term.buffer.active;
  draftView.style.left = Math.min(draftAnchor?.x ?? cursor.cursorX, term.cols - 1) * 100 / term.cols + '%';
  draftView.style.top = Math.min(draftAnchor?.y ?? cursor.cursorY, term.rows - 1) * 100 / term.rows + '%';
  draftView.style.height = 100 / term.rows + '%';
}
term.onRender(positionDraft);

function finishComposition(clearInput) {
  if (!settling) return;
  const composition = settling;
  settling = null;
  const value = term.textarea.value;
  const text = value.slice(composition.start, value.length - composition.tail);
  replaceDraft(composition, text);
  if (clearInput) term.textarea.value = '';
}

// Only the Galaxy IDE text cursor opts into live edits. Selections and other TUIs get a preview.
host.addEventListener('compositionstart', event => {
  if (event.target !== term.textarea) return;
  imeInputPending = false;
  editorCommit = null;
  finishComposition(false);
  const editor = inEditorScreen();
  if (!running || (!atPrompt && !editor)) return;
  control.setAttribute('aria-pressed', 'false');
  composing = {
    editor,
    live: editor && liveEditor,
    preview: editor && !liveEditor,
    text: '',
    start: term.textarea.selectionStart,
    tail: term.textarea.value.length - term.textarea.selectionEnd
  };
  event.stopPropagation();
}, true);
host.addEventListener('compositionupdate', event => {
  if (!composing) return;
  event.stopPropagation();
  if (composing.live) return; // The following input event contains the updated textarea.
  if (/[\x00-\x1f\x7f]/.test(event.data)) return;
  if (composing.preview) {
    composing.text = event.data;
    draftView.textContent = event.data;
    draftView.hidden = !event.data;
    positionDraft();
  } else {
    replaceDraft(composing, event.data);
  }
}, true);
host.addEventListener('compositionend', event => {
  if (!composing) return;
  event.stopPropagation();
  if (composing.live) {
    composing = null;
    syncLiveRange();
    return;
  }
  if (composing.editor) {
    const finalText = event.data || '';
    if (composing.preview) {
      draftView.hidden = true;
      if (finalText) term.input(finalText);
    } else {
      replaceDraft(composing, finalText);
    }
    // Samsung can follow a commit with an input notification for the same edit.
    editorCommit = { text: finalText, value: term.textarea.value.slice(0, composing.start) + finalText +
      (composing.tail ? term.textarea.value.slice(-composing.tail) : '') };
    composing = null;
    return;
  }
  const composition = settling = composing;
  composing = null;
  // Chromium can update the textarea after compositionend.
  setTimeout(() => { if (settling === composition) finishComposition(true); }, 0);
}, true);
host.addEventListener('input', event => {
  if (event.target !== term.textarea) return;
  if (liveEditor && (!composing || composing.live)) {
    event.stopPropagation();
    syncLiveRange();
    return;
  }
  if (composing || settling) {
    event.stopPropagation();
    return;
  }
  if (editorCommit && event.data === editorCommit.text && term.textarea.value === editorCommit.value &&
      (event.inputType === 'insertText' || event.inputType === 'insertCompositionText')) {
    imeInputPending = false;
    event.stopPropagation();
    return;
  }
  const editorText = inEditorScreen() && event.inputType === 'insertText';
  if (!imeInputPending && !editorText) return;
  imeInputPending = false;
  event.stopPropagation();
  let data = event.data;
  if (event.inputType === 'deleteContentBackward') data = '\x7f';
  if (event.inputType === 'insertLineBreak') data = '\r';
  if (data) term.input(data);
  term.textarea.value = '';
}, true);
host.addEventListener('keydown', event => {
  if (event.target !== term.textarea) return;
  if (inGalaxyIDE && event.key === 'Escape' && !event.altKey && !event.ctrlKey && !event.metaKey) {
    event.preventDefault();
    event.stopPropagation();
    editorEscape();
    return;
  }
  const processKey = event.keyCode === 229;
  if (!processKey && event.key !== 'HangulMode' && !['Shift', 'Control', 'Alt', 'Meta', 'CapsLock'].includes(event.key)) editorCommit = null;
  if (liveEditor && !composing && !processKey && (event.ctrlKey || event.metaKey || event.altKey ||
      /^(Arrow|Page|F\d+$)/.test(event.key) || ['Home', 'End', 'Enter', 'Tab', 'Escape', 'Backspace', 'Delete'].includes(event.key))) {
    resetLiveRange();
  }
  if (composing || (settling && (event.isComposing || processKey))) {
    event.stopPropagation();
    return;
  }
  finishComposition(true);
  imeInputPending = running && (atPrompt || inEditorScreen()) && processKey;
  // A Process key can arrive before compositionstart. xterm would read and resend the first draft.
  if (imeInputPending || editorPrintable(event)) event.stopPropagation();
}, true);

// Let Chromium turn physical printable keys into IME text before forwarding them.
function editorPrintable(event) {
  return inEditorScreen() && event.key?.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey;
}
host.addEventListener('keypress', event => {
  if (event.target === term.textarea && (composing || editorPrintable(event))) event.stopPropagation();
}, true);
host.addEventListener('pointerdown', () => {
  if (nativeTerminal) Android.resetEditorContext?.();
  if (liveEditor && !composing) {
    resetLiveRange();
    Android.resetEditorContext?.();
  }
}, true);
host.addEventListener('pointerup', () => {
  updateInputMode();
});

function closeMenu() {
  menu.hidden = true;
  clipboard.setAttribute('aria-expanded', 'false');
}

function openMenu(x, y, focus) {
  copy.disabled = !term.hasSelection();
  paste.disabled = !running;
  clipboard.setAttribute('aria-expanded', 'true');
  menu.hidden = false;
  menu.style.left = `${Math.max(6, Math.min(x, innerWidth - menu.offsetWidth - 6))}px`;
  menu.style.top = `${Math.max(6, Math.min(y, innerHeight - menu.offsetHeight - 6))}px`;
  if (focus) menu.querySelector('button:not(:disabled)')?.focus();
}

term.attachCustomKeyEventHandler(event => {
  if (!event.ctrlKey || !event.shiftKey || event.altKey || event.metaKey) return true;
  if (event.code !== 'KeyC' && event.code !== 'KeyV') return true;
  event.preventDefault();
  if (event.type === 'keydown') {
    if (event.code === 'KeyC' && term.hasSelection()) Android.copy(term.getSelection());
    if (event.code === 'KeyV' && running) Android.paste();
  }
  return false;
});

function pasteText(text) {
  if (!running || composing) return;
  finishComposition(true);
  control.setAttribute('aria-pressed', 'false');
  term.paste(text);
}

host.addEventListener('mousedown', event => {
  if (event.button === 2) event.stopPropagation();
}, true);
host.addEventListener('contextmenu', event => {
  event.preventDefault();
  event.stopPropagation();
  openMenu(event.clientX, event.clientY, false);
}, true);
clipboard.addEventListener('click', () => {
  if (!menu.hidden) return closeMenu();
  const anchor = clipboard.getBoundingClientRect();
  openMenu(anchor.left, anchor.bottom + 4, true);
});
document.addEventListener('pointerdown', event => {
  if (!menu.contains(event.target) && !clipboard.contains(event.target)) closeMenu();
}, true);
document.addEventListener('keydown', event => {
  if (!menu.hidden && event.key === 'Escape') {
    closeMenu();
    term.focus();
    event.stopPropagation();
  }
}, true);
copy.addEventListener('click', () => {
  if (term.hasSelection()) Android.copy(term.getSelection());
  closeMenu();
  term.focus();
});
paste.addEventListener('click', () => {
  Android.paste();
  closeMenu();
  term.focus();
});

document.getElementById('keyboard').addEventListener('click', () => {
  term.focus();
  Android.keyboard();
});
restart.addEventListener('click', () => {
  term.reset();
  openShell();
  term.focus();
});
document.getElementById('keys').addEventListener('pointerdown', event => {
  if (event.target instanceof HTMLButtonElement) event.preventDefault();
});
document.getElementById('keys').addEventListener('click', event => {
  const key = event.target.dataset.key;
  if (!running || (!key && event.target !== control)) return;
  term.focus();
  if (key) {
    if (key === 'Escape' && inGalaxyIDE) return editorEscape();
    if (composing) return;
    if (liveEditor || nativeTerminal) Android.resetEditorContext?.();
    finishComposition(true);
    if (key === 'Escape') term.input('\x1b');
    else if (key === 'Tab') term.input('\t');
    else term.input(`\x1b${term.modes.applicationCursorKeysMode ? 'O' : '['}${cursorKeys[key]}`);
  } else {
    control.setAttribute('aria-pressed', String(control.getAttribute('aria-pressed') !== 'true'));
  }
});

term.options.disableStdin = true;
openShell();
