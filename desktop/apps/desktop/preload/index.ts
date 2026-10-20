import { contextBridge, ipcRenderer } from 'electron'
import type { DesktopAPI, DesktopEvent } from '../../../packages/protocol/src/index'

const api: DesktopAPI = {
  hello: () => ipcRenderer.invoke('tcview:hello'),
  listRuns: () => ipcRenderer.invoke('tcview:list'),
  listDeletedRuns: () => ipcRenderer.invoke('tcview:trash'),
  deleteRun: runId => ipcRenderer.invoke('tcview:delete', runId),
  restoreRun: runId => ipcRenderer.invoke('tcview:restore', runId),
  startRun: input => ipcRenderer.invoke('tcview:start', input),
  getRun: runId => ipcRenderer.invoke('tcview:get', runId),
  stopRun: runId => ipcRenderer.invoke('tcview:stop', runId),
  resumeRun: input => ipcRenderer.invoke('tcview:resume', input),
  getEvents: (runId, afterEventId) => ipcRenderer.invoke('tcview:events', { runId, afterEventId }),
  listProviders: () => ipcRenderer.invoke('tcview:providers'),
  checkProvider: (providerId, config) => ipcRenderer.invoke('tcview:provider-check', {providerId, config}),
  getProviderSettings: () => ipcRenderer.invoke('tcview:provider-settings-get'),
  saveProviderSettings: configs => ipcRenderer.invoke('tcview:provider-settings-save', configs),
  resetProviderSettings: () => ipcRenderer.invoke('tcview:provider-settings-reset'),
  reloadProviderModels: providerId => ipcRenderer.invoke('tcview:provider-models', providerId),
  branchRun: input => ipcRenderer.invoke('tcview:branch', input),
  exportArtifact: (runId, format) => ipcRenderer.invoke('tcview:export', { runId, format }),
  reconnectHost: () => ipcRenderer.invoke('tcview:reconnect'),
  clearDiagnostics: runId => ipcRenderer.invoke('tcview:diagnostics-clear', runId),
  importMaterials: () => ipcRenderer.invoke('tcview:materials-import'),
  analyzeTopic: input => ipcRenderer.invoke('tcview:topic-analyze', input),
  getPreparation: id => ipcRenderer.invoke('tcview:topic-get', id),
  cancelPreparation: id => ipcRenderer.invoke('tcview:topic-cancel', id),
  subscribe: listener => {
    const wrapped = (_event: unknown, value: DesktopEvent) => listener(value)
    ipcRenderer.on('tcview:event', wrapped)
    return () => { ipcRenderer.removeListener('tcview:event', wrapped) }
  },
}

contextBridge.exposeInMainWorld('tcview', api)

