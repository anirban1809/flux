'use strict';

// ---- State ----

let port = 7701;
let activeSessionId = null;
let isRunning = false;

// ---- DOM refs ----

const sessionList = document.getElementById('session-list');
const messages = document.getElementById('messages');
const emptyState = document.getElementById('empty-state');
const sessionPath = document.getElementById('session-path');
const promptInput = document.getElementById('prompt-input');
const sendBtn = document.getElementById('send-btn');
const clearBtn = document.getElementById('clear-btn');
const newSessionBtn = document.getElementById('btn-new-session');

// ---- API helpers ----

function apiUrl(path) {
  return `http://127.0.0.1:${port}${path}`;
}

async function apiFetch(path, opts = {}) {
  const res = await fetch(apiUrl(path), {
    headers: { 'Content-Type': 'application/json', ...opts.headers },
    ...opts,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`API ${opts.method || 'GET'} ${path} → ${res.status}: ${text}`);
  }
  return res;
}

// ---- Initialisation ----

async function init() {
  port = await window.flux.getDaemonPort();

  // Try to load existing sessions
  try {
    const res = await apiFetch('/api/sessions');
    const sessions = await res.json();
    sessions.forEach(renderSessionItem);
    if (sessions.length > 0) {
      selectSession(sessions[0]);
    }
  } catch (e) {
    console.error('Failed to load sessions:', e);
  }
}

// ---- Session management ----

async function createSession() {
  const cwd = await window.flux.getCwd();
  const res = await apiFetch('/api/sessions', {
    method: 'POST',
    body: JSON.stringify({ work_dir: cwd }),
  });
  const session = await res.json();
  renderSessionItem(session);
  selectSession(session);
}

function renderSessionItem(session) {
  // Avoid duplicates
  if (document.getElementById(`session-${session.id}`)) return;

  const el = document.createElement('div');
  el.className = 'session-item';
  el.id = `session-${session.id}`;
  el.dataset.id = session.id;
  el.dataset.workDir = session.work_dir;

  const name = document.createElement('div');
  name.className = 'session-name';
  name.textContent = shortName(session.work_dir);

  const dir = document.createElement('div');
  dir.className = 'session-dir';
  dir.textContent = session.work_dir;

  el.appendChild(name);
  el.appendChild(dir);
  el.addEventListener('click', () => selectSession(session));
  sessionList.appendChild(el);
}

function selectSession(session) {
  // Update active highlight
  document.querySelectorAll('.session-item').forEach((el) => {
    el.classList.toggle('active', el.dataset.id === session.id);
  });

  activeSessionId = session.id;
  sessionPath.textContent = session.work_dir;
  clearMessages();
  sendBtn.disabled = false;
  promptInput.disabled = false;
  promptInput.focus();
}

function shortName(workDir) {
  const parts = workDir.replace(/^\//, '').split('/');
  return parts[parts.length - 1] || workDir;
}

// ---- Messages ----

function clearMessages() {
  messages.innerHTML = '';
  emptyState.style.display = 'none';
}

function appendMessage(type, text) {
  const wrapper = document.createElement('div');
  wrapper.className = `msg-${type}`;

  const bubble = document.createElement('div');
  bubble.className = 'bubble';

  if (type === 'tool') {
    const label = document.createElement('div');
    label.className = 'tool-label';
    label.textContent = '⚙ tool';
    bubble.appendChild(label);
  }

  const content = document.createElement('span');
  content.textContent = text;
  bubble.appendChild(content);
  wrapper.appendChild(bubble);
  messages.appendChild(wrapper);
  messages.scrollTop = messages.scrollHeight;
  return content; // return span so we can update it for streaming
}

function appendStreamingMessage() {
  const wrapper = document.createElement('div');
  wrapper.className = 'msg-assistant';
  const bubble = document.createElement('div');
  bubble.className = 'bubble cursor';
  wrapper.appendChild(bubble);
  messages.appendChild(wrapper);
  messages.scrollTop = messages.scrollHeight;
  return { bubble, wrapper };
}

// ---- Send ----

async function sendMessage() {
  if (!activeSessionId || isRunning) return;
  const prompt = promptInput.value.trim();
  if (!prompt) return;

  promptInput.value = '';
  promptInput.style.height = 'auto';
  setRunning(true);

  appendMessage('user', prompt);

  // Streaming assistant message
  const { bubble, wrapper } = appendStreamingMessage();
  let accumulatedText = '';
  let lastType = '';

  try {
    const res = await fetch(apiUrl(`/api/sessions/${activeSessionId}/run`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt }),
    });

    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const blocks = buffer.split('\n\n');
      buffer = blocks.pop(); // keep incomplete block

      for (const block of blocks) {
        for (const line of block.split('\n')) {
          if (!line.startsWith('data: ')) continue;
          let event;
          try {
            event = JSON.parse(line.slice(6));
          } catch (_) {
            continue;
          }
          handleSSEEvent(event, bubble, wrapper);
        }
      }

      messages.scrollTop = messages.scrollHeight;
    }
  } catch (err) {
    bubble.classList.remove('cursor');
    wrapper.classList.remove('msg-assistant');
    wrapper.classList.add('msg-error');
    bubble.textContent = `Error: ${err.message}`;
  } finally {
    bubble.classList.remove('cursor');
    setRunning(false);
  }
}

function handleSSEEvent(event, bubble, wrapper) {
  switch (event.type) {
    case 'chunk':
      bubble.textContent += event.text;
      break;

    case 'message': {
      // Replace streaming bubble with final message
      bubble.classList.remove('cursor');
      bubble.textContent = event.text;
      break;
    }

    case 'tool': {
      // Finalise current streaming bubble if it has content
      if (bubble.textContent) {
        bubble.classList.remove('cursor');
      }
      // Render tool event separately
      appendMessage('tool', event.text);
      break;
    }

    case 'error':
      bubble.classList.remove('cursor');
      wrapper.classList.remove('msg-assistant');
      wrapper.classList.add('msg-error');
      bubble.textContent = event.error || event.text;
      break;

    case 'done':
      bubble.classList.remove('cursor');
      // If bubble is still empty (tool-only run), remove it
      if (!bubble.textContent) {
        wrapper.remove();
      }
      break;
  }
}

// ---- UI state ----

function setRunning(running) {
  isRunning = running;
  sendBtn.disabled = running;
  sendBtn.textContent = running ? '…' : 'Send';
  promptInput.disabled = running;
}

// ---- Event listeners ----

newSessionBtn.addEventListener('click', createSession);

clearBtn.addEventListener('click', async () => {
  if (!activeSessionId || isRunning) return;
  await apiFetch(`/api/sessions/${activeSessionId}/clear`, { method: 'POST' });
  clearMessages();
});

sendBtn.addEventListener('click', sendMessage);

promptInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMessage();
  }
});

// Auto-resize textarea
promptInput.addEventListener('input', () => {
  promptInput.style.height = 'auto';
  promptInput.style.height = Math.min(promptInput.scrollHeight, 200) + 'px';
});

// ---- Boot ----

init().catch(console.error);
