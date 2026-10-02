import type { Command, CommandResponse, ServerMessage, Snapshot } from '../types'

export class ApiError extends Error {
  constructor(message: string, readonly status: number) { super(message) }
}

async function parse<T>(response: Response): Promise<T> {
  const body = await response.json().catch(() => ({})) as T & { error?: string; reason?: string }
  if (!response.ok) throw new ApiError(body.reason ?? body.error ?? `Request failed (${response.status})`, response.status)
  return body
}

export const sessionApi = {
  get: (signal?: AbortSignal) => fetch('/api/session', { signal, headers: { Accept: 'application/json' } }).then(parse<Snapshot>),
  command: (command: Command) => fetch('/api/command', {
    method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify(command),
  }).then(parse<CommandResponse>),
  exportUrl: (sessionId: string) => `/api/export?session=${encodeURIComponent(sessionId)}`,
  import: (file: File) => fetch('/api/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: file }).then(parse<Snapshot>),
}

export class SessionSocket {
  private socket?: WebSocket
  private reconnectTimer?: ReturnType<typeof setTimeout>
  private attempts = 0
  private closed = false

  constructor(private sessionId: string, private onMessage: (message: ServerMessage) => void, private onState: (state: 'connected' | 'reconnecting' | 'disconnected') => void) {}

  connect() {
    this.closed = false
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.socket = new WebSocket(`${protocol}//${location.host}/ws?session=${encodeURIComponent(this.sessionId)}`)
    this.socket.addEventListener('open', () => { this.attempts = 0; this.onState('connected') })
    this.socket.addEventListener('message', event => {
      try { this.onMessage(JSON.parse(event.data as string) as ServerMessage) } catch { /* ignore malformed server frames */ }
    })
    this.socket.addEventListener('close', () => {
      if (this.closed) { this.onState('disconnected'); return }
      this.onState('reconnecting')
      this.attempts++
      this.reconnectTimer = setTimeout(() => this.connect(), Math.min(500 * 2 ** this.attempts, 10_000))
    })
    this.socket.addEventListener('error', () => this.socket?.close())
  }

  close() { this.closed = true; clearTimeout(this.reconnectTimer); this.socket?.close() }
}
