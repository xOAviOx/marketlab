import type { CommandResponse, ServerMessage, SessionCommand, SessionSnapshot } from '../types'

export class ApiError extends Error {
  constructor(message: string, readonly status: number) { super(message) }
}

async function parse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await response.json().catch(() => ({ message: response.statusText })) as { message?: string }
    throw new ApiError(body.message ?? `Request failed (${response.status})`, response.status)
  }
  return response.json() as Promise<T>
}

export const sessionApi = {
  get: (signal?: AbortSignal) => fetch('/api/session', { signal, headers: { Accept: 'application/json' } }).then(parse<SessionSnapshot>),
  command: (sessionId: string, command: SessionCommand, requestId = crypto.randomUUID()) =>
    fetch('/api/command', {
      method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ sessionId, requestId, command }),
    }).then(parse<CommandResponse>),
  exportUrl: (sessionId: string) => `/api/export?session=${encodeURIComponent(sessionId)}`,
  import: (file: File) => {
    const form = new FormData()
    form.append('session', file)
    return fetch('/api/import', { method: 'POST', body: form }).then(parse<SessionSnapshot>)
  },
}

export class SessionSocket {
  private socket?: WebSocket
  private reconnectTimer?: ReturnType<typeof setTimeout>
  private attempts = 0
  private closed = false

  constructor(
    private sessionId: string,
    private onMessage: (message: ServerMessage) => void,
    private onState: (state: 'connected' | 'reconnecting' | 'disconnected') => void,
  ) {}

  connect() {
    this.closed = false
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.socket = new WebSocket(`${protocol}//${location.host}/ws?session=${encodeURIComponent(this.sessionId)}`)
    this.socket.addEventListener('open', () => { this.attempts = 0; this.onState('connected') })
    this.socket.addEventListener('message', event => {
      try { this.onMessage(JSON.parse(event.data as string) as ServerMessage) } catch { /* malformed frames are ignored */ }
    })
    this.socket.addEventListener('close', () => {
      if (this.closed) { this.onState('disconnected'); return }
      this.onState('reconnecting')
      this.attempts += 1
      this.reconnectTimer = setTimeout(() => this.connect(), Math.min(1000 * 2 ** this.attempts, 15000))
    })
    this.socket.addEventListener('error', () => this.socket?.close())
  }

  close() {
    this.closed = true
    clearTimeout(this.reconnectTimer)
    this.socket?.close()
  }
}
