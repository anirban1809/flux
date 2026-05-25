const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('flux', {
  getDaemonPort: () => ipcRenderer.invoke('get-daemon-port'),
  getCwd: () => ipcRenderer.invoke('get-cwd'),
});
