const { app, BrowserWindow, ipcMain, shell } = require('electron');
const { spawn, execSync } = require('child_process');
const path = require('path');
const http = require('http');
const fs = require('fs');

const DAEMON_PORT = 7701;
let daemonProcess = null;
let mainWindow = null;

// ------- Daemon binary location -------

function findFluxBinary() {
  // Development: repo/bin/flux (built via `make build`)
  const devPath = path.join(__dirname, '..', 'bin', 'flux');
  if (fs.existsSync(devPath)) return devPath;

  // Installed via `make build`
  const home = process.env.HOME || '';
  const installPath = path.join(home, '.flux', 'bin', 'flux');
  if (fs.existsSync(installPath)) return installPath;

  // Fall back to PATH
  try {
    const which = execSync('which flux', { encoding: 'utf8' }).trim();
    if (which) return which;
  } catch (_) {}

  return null;
}

// ------- Daemon lifecycle -------

function daemonUrl(path) {
  return `http://127.0.0.1:${DAEMON_PORT}${path}`;
}

function checkDaemon() {
  return new Promise((resolve) => {
    http.get(daemonUrl('/api/health'), (res) => {
      resolve(res.statusCode === 200);
    }).on('error', () => resolve(false));
  });
}

async function waitForDaemon(timeoutMs = 6000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await checkDaemon()) return;
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('flux daemon failed to start within timeout');
}

async function startDaemon(binaryPath) {
  if (await checkDaemon()) {
    console.log('[flux] daemon already running');
    return;
  }

  console.log('[flux] starting daemon:', binaryPath);
  daemonProcess = spawn(binaryPath, ['--daemon', '--port', String(DAEMON_PORT)], {
    detached: false,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  daemonProcess.stdout.on('data', (d) => process.stdout.write('[daemon] ' + d));
  daemonProcess.stderr.on('data', (d) => process.stderr.write('[daemon] ' + d));

  daemonProcess.on('exit', (code) => {
    console.log('[flux] daemon exited with code', code);
    daemonProcess = null;
  });

  await waitForDaemon();
}

// ------- Window -------

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 800,
    minHeight: 600,
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    backgroundColor: '#0d1117',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadFile(path.join(__dirname, 'src', 'index.html'));

  if (process.argv.includes('--dev')) {
    mainWindow.webContents.openDevTools();
  }

  mainWindow.on('closed', () => { mainWindow = null; });
}

function showError(message) {
  mainWindow = new BrowserWindow({ width: 640, height: 320, backgroundColor: '#0d1117' });
  mainWindow.loadURL(
    `data:text/html;charset=utf-8,` +
    encodeURIComponent(`<!DOCTYPE html><html><body style="font-family:monospace;color:#e6edf3;background:#0d1117;padding:2rem">` +
      `<h2 style="color:#f85149">⚡ flux — startup error</h2><p>${message}</p>` +
      `<p style="color:#8b949e">Build the binary first: <code>make build</code></p></body></html>`)
  );
}

// ------- IPC handlers -------

ipcMain.handle('get-daemon-port', () => DAEMON_PORT);
ipcMain.handle('get-cwd', () => process.cwd());

// ------- App lifecycle -------

app.whenReady().then(async () => {
  const binary = findFluxBinary();
  if (!binary) {
    createWindow(); // create window first so showError works
    showError('flux binary not found. Run <code>make build</code> from the repo root.');
    return;
  }

  try {
    await startDaemon(binary);
    createWindow();
  } catch (err) {
    createWindow();
    showError(`Failed to start daemon: ${err.message}`);
  }
});

app.on('window-all-closed', () => {
  if (daemonProcess) {
    daemonProcess.kill();
    daemonProcess = null;
  }
  if (process.platform !== 'darwin') app.quit();
});

app.on('activate', () => {
  if (!mainWindow) createWindow();
});
