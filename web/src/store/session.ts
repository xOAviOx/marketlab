import { create } from 'zustand'
import { sessionApi, SessionSocket } from '../api/client'
import { demoSnapshot } from '../data/demo'
import type { ConnectionState, ServerMessage, SessionCommand, SessionSnapshot } from '../types'

interface Notice { id: string; kind: 'success' | 'error' | 'info'; message: string }

interface SessionStore {
  snapshot: SessionSnapshot | null
  connection: ConnectionState
  demoMode: boolean
  pending: boolean
  notice: Notice | null
  initialize: () => () => void
  send: (command: SessionCommand) => Promise<boolean>
  importSession: (file: File) => Promise<boolean>
  clearNotice: () => void
  setSnapshot: (snapshot: SessionSnapshot) => void
}

const messageToSnapshot = (message: ServerMessage) => message.type === 'snapshot' || message.type === 'patch' ? message.snapshot : null

export const useSessionStore = create<SessionStore>((set, get) => ({
  snapshot: null,
  connection: 'loading',
  demoMode: false,
  pending: false,
  notice: null,
  initialize: () => {
    const controller = new AbortController()
    let socket: SessionSocket | undefined
    sessionApi.get(controller.signal).then(snapshot => {
      set({ snapshot, connection: 'connected', demoMode: false })
      socket = new SessionSocket(snapshot.sessionId, message => {
        const next = messageToSnapshot(message)
        if (next && next.sequence >= (get().snapshot?.sequence ?? 0)) set({ snapshot: next })
        if (message.type === 'command.rejected') set({ notice: { id: message.requestId, kind: 'error', message: message.reason } })
      }, connection => set({ connection }))
      socket.connect()
    }).catch(error => {
      if (error instanceof DOMException && error.name === 'AbortError') return
      set({ snapshot: demoSnapshot, connection: 'disconnected', demoMode: true, notice: { id: 'offline', kind: 'info', message: 'Backend unavailable — showing a read-only demo snapshot.' } })
    })
    return () => { controller.abort(); socket?.close() }
  },
  send: async command => {
    const snapshot = get().snapshot
    if (!snapshot) return false
    set({ pending: true })
    try {
      const result = await sessionApi.command(snapshot.sessionId, command)
      if (!result.accepted) throw new Error(result.reason ?? 'Command rejected')
      set({ pending: false, snapshot: result.snapshot ?? get().snapshot, notice: { id: result.requestId, kind: 'success', message: 'Command accepted' } })
      return true
    } catch (error) {
      set({ pending: false, notice: { id: crypto.randomUUID(), kind: 'error', message: error instanceof Error ? error.message : 'Command rejected' } })
      return false
    }
  },
  importSession: async file => {
    set({ pending: true })
    try {
      const snapshot = await sessionApi.import(file)
      set({ pending: false, snapshot, demoMode: false, notice: { id: crypto.randomUUID(), kind: 'success', message: `Imported ${file.name}` } })
      return true
    } catch (error) {
      set({ pending: false, notice: { id: crypto.randomUUID(), kind: 'error', message: error instanceof Error ? error.message : 'Import failed' } })
      return false
    }
  },
  clearNotice: () => set({ notice: null }),
  setSnapshot: snapshot => set({ snapshot }),
}))
