import { Download, Pause, Play, RotateCcw, Upload } from 'lucide-react'
import { useRef } from 'react'
import { sessionApi } from '../api/client'
import { useSessionStore } from '../store/session'
import { Button, Tip } from './ui'

const speeds = [.5, 1, 2, 4]

export function SimulationBar() {
  const simulation = useSessionStore(s => s.snapshot?.simulation)
  const sessionId = useSessionStore(s => s.snapshot?.sessionId)
  const pending = useSessionStore(s => s.pending)
  const send = useSessionStore(s => s.send)
  const importSession = useSessionStore(s => s.importSession)
  const fileRef = useRef<HTMLInputElement>(null)
  if (!simulation || !sessionId) return null
  const mins = (value: number) => `${Math.floor(value / 60).toString().padStart(2, '0')}:${Math.floor(value % 60).toString().padStart(2, '0')}`
  return <div className="simulation-bar">
    <div className="sim-controls">
      <Tip label={simulation.state === 'running' ? 'Pause simulation' : 'Resume simulation'}><Button aria-label={simulation.state === 'running' ? 'Pause simulation' : 'Resume simulation'} disabled={pending} onClick={() => void send({ type: simulation.state === 'running' ? 'simulation.pause' : 'simulation.resume' })}>{simulation.state === 'running' ? <Pause size={14} /> : <Play size={14} />}</Button></Tip>
      <Tip label="Restart from the beginning"><Button aria-label="Restart simulation" disabled={pending} onClick={() => void send({ type: 'simulation.restart' })}><RotateCcw size={14} /></Button></Tip>
      <span className={`run-state ${simulation.state}`}>{simulation.state}</span>
    </div>
    <div className="timeline-control">
      <span>{mins(simulation.elapsed)}</span>
      <input aria-label="Replay timeline" type="range" min={0} max={simulation.duration} value={simulation.elapsed} onChange={event => void send({ type: 'simulation.seek', elapsed: Number(event.target.value) })} />
      <span>{mins(simulation.duration)}</span>
    </div>
    <div className="speed-control" aria-label="Simulation speed">
      {speeds.map(speed => <button key={speed} className={simulation.speed === speed ? 'active' : ''} onClick={() => void send({ type: 'simulation.speed', speed })}>{speed}×</button>)}
    </div>
    <div className="file-controls">
      <Tip label="Import a MarketLab session"><Button aria-label="Import session" onClick={() => fileRef.current?.click()}><Upload size={14} /></Button></Tip>
      <input ref={fileRef} hidden type="file" accept="application/json,.json" onChange={event => { const file = event.target.files?.[0]; if (file) void importSession(file) }} />
      <Tip label="Export this session"><a className="button" aria-label="Export session" download={`marketlab-${sessionId}.json`} href={sessionApi.exportUrl(sessionId)}><Download size={14} /></a></Tip>
    </div>
  </div>
}
