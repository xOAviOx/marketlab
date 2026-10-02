import { create } from 'zustand'
import { sessionApi, SessionSocket } from '../api/client'
import type { Command, ConnectionState, Snapshot } from '../types'

interface Notice { kind: 'success' | 'error' | 'info'; message: string }
interface SessionStore {
  snapshot: Snapshot | null
  connection: ConnectionState
  pending: boolean
  notice: Notice | null
  initialize: () => () => void
  send: (command: Command, success?: string) => Promise<boolean>
  importExperiment: (file: File) => Promise<boolean>
  clearNotice: () => void
}

export const useSessionStore = create<SessionStore>((set, get) => ({
  snapshot: null, connection: 'loading', pending: false, notice: null,
  initialize: () => {
    const controller = new AbortController()
    let socket: SessionSocket | undefined
    sessionApi.get(controller.signal).then(snapshot => {
      set({ snapshot, connection: 'connected' })
      socket = new SessionSocket(snapshot.sessionId, message => {
        if (message.type === 'snapshot') set({ snapshot: message.snapshot })
      }, connection => set({ connection }))
      socket.connect()
    }).catch(error => {
      if (error instanceof DOMException && error.name === 'AbortError') return
      set({ connection: 'disconnected', notice: { kind: 'error', message: 'Backend unavailable. Reconnect to receive an authoritative market snapshot.' } })
    })
    return () => { controller.abort(); socket?.close() }
  },
  send: async (command, success = 'Command accepted') => {
    if (!get().snapshot) return false
    set({ pending: true })
    try {
      const result = await sessionApi.command(command)
      set({ pending: false, snapshot: result.snapshot, notice: { kind: 'success', message: success } })
      return true
    } catch (error) {
      set({ pending: false, notice: { kind: 'error', message: error instanceof Error ? error.message : 'Command rejected' } })
      return false
    }
  },
  importExperiment: async file => {
    set({ pending: true })
    try {
      const snapshot = await sessionApi.import(file)
      set({ pending: false, snapshot, notice: { kind: 'success', message: `Imported ${file.name} in read-only replay mode.` } })
      return true
    } catch (error) {
      set({ pending: false, notice: { kind: 'error', message: error instanceof Error ? error.message : 'Import failed' } })
      return false
    }
  },
  clearNotice: () => set({ notice: null }),
}))
