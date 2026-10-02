import { Activity, AlertTriangle, Bot, ChevronRight, CircleDollarSign, CloudOff, Download, Gauge, Info, Pause, Play, RotateCcw, StepForward, Upload, X, Zap } from 'lucide-react'
import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { sessionApi } from './api/client'
import { MarketChart } from './components/Chart'
import { useSessionStore } from './store/session'
import type { Bot as BotModel, Level, OrderType, ScenarioEvent, Side } from './types'

const usd = (cents: number, sign = false) => `${sign && cents > 0 ? '+' : cents < 0 ? '−' : ''}$${(Math.abs(cents) / 100).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
const integer = (value: number) => value.toLocaleString('en-US')
const simTime = (ms: number) => `${Math.floor(ms / 60_000).toString().padStart(2, '0')}:${Math.floor((ms % 60_000) / 1000).toString().padStart(2, '0')}.${Math.floor(ms % 1000 / 100)}`

function Panel({ title, eyebrow, children, className = '', action }: { title: string; eyebrow?: string; children: React.ReactNode; className?: string; action?: React.ReactNode }) {
  return <section className={`panel ${className}`}><header className="panel-head"><div>{eyebrow && <span>{eyebrow}</span>}<h2>{title}</h2></div>{action}</header>{children}</section>
}

function Loading() {
  return <main className="loading"><div className="logo">M<span>L</span></div><div><strong>Opening MarketLab</strong><p>Establishing an isolated simulated exchange…</p></div><i /></main>
}

function SimulationControls() {
  const snapshot = useSessionStore(s => s.snapshot!)
  const pending = useSessionStore(s => s.pending)
  const send = useSessionStore(s => s.send)
  const [seed, setSeed] = useState(String(snapshot.seed))
  return <div className="simbar">
    <div className="transport">
      <button className="icon-button primary" disabled={pending} aria-label={snapshot.running ? 'Pause simulation' : 'Start simulation'} onClick={() => void send({ type: snapshot.running ? 'simulation.pause' : 'simulation.start' }, snapshot.running ? 'Simulation paused' : 'Simulation started')} title={snapshot.running ? 'Pause' : 'Start'}>{snapshot.running ? <Pause /> : <Play />}</button>
      <button className="icon-button" disabled={pending || snapshot.running} aria-label="Advance one simulation step" onClick={() => void send({ type: 'simulation.step' }, 'Advanced one logical step')} title="Single step"><StepForward /></button>
      <span className={`state-pill ${snapshot.running ? 'running' : ''}`}><i />{snapshot.running ? 'RUNNING' : 'PAUSED'}</span>
    </div>
    <div className="sim-clock"><span>ENGINE TIME</span><strong>{simTime(snapshot.logicalTime)}</strong></div>
    <div className="speed" aria-label="Playback speed">{[.5, 1, 2, 4].map(speed => <button key={speed} className={snapshot.speed === speed ? 'active' : ''} onClick={() => void send({ type: 'simulation.speed', speed }, `Speed set to ${speed}×`)}>{speed}×</button>)}</div>
    <form className="seed" onSubmit={event => { event.preventDefault(); const value = Number(seed); if (Number.isSafeInteger(value)) void send({ type: 'simulation.restart', seed: value }, `Reset with seed ${value}`) }}><label><span>SEED</span><input value={seed} onChange={event => setSeed(event.target.value)} inputMode="numeric" aria-label="Simulation seed" /></label><button className="icon-button" aria-label="Reset simulation with seed" title="Reset with seed"><RotateCcw /></button></form>
  </div>
}

function MarketStrip() {
  const market = useSessionStore(s => s.snapshot!.market)
  const cells = [
    ['LAST', usd(market.lastPrice), market.markSource],
    ['RUN CHANGE', usd(market.runChange, true), 'From the $100.00 initial reference'],
    ['SPREAD', market.spread ? `${market.spread}¢` : '—', 'Best ask minus best bid'],
    ['EXECUTED VOLUME', `${integer(market.volume)} sh`, 'Matched shares in this run'],
  ]
  return <div className="market-strip">{cells.map(([label, value, hint], i) => <div key={label}><span>{label}{(i === 2 || i === 3) && <i className="info-tip" aria-label={hint} title={hint}><Info /></i>}</span><strong className={i === 1 ? market.runChange >= 0 ? 'buy' : 'sell' : ''}>{value}</strong><small>{hint}</small></div>)}</div>
}

function Depth({ levels, side }: { levels: Level[]; side: 'bid' | 'ask' }) {
  const max = Math.max(1, ...levels.map(level => level.quantity))
  if (!levels.length) return <div className="empty">No resting {side === 'bid' ? 'bids' : 'asks'}</div>
  return <div className={`depth ${side}`}>{levels.slice(0, 10).map(level => <div className="depth-row" key={level.price}><i style={{ width: `${level.quantity / max * 100}%` }} /><span>{usd(level.price)}</span><strong>{integer(level.quantity)}</strong><small>{level.orders}</small></div>)}</div>
}

function MarketDepth() {
  const book = useSessionStore(s => s.snapshot!.book)
  return <Panel title="Order book" eyebrow="LIVE DEPTH" className="book-panel"><div className="table-head"><span>PRICE</span><span>QTY</span><span>ORD</span></div><Depth levels={[...book.asks].reverse()} side="ask" /><div className="midline"><span>ASK</span><span>MARKET</span><span>BID</span></div><Depth levels={book.bids} side="bid" /></Panel>
}

function TradeTape() {
  const trades = useSessionStore(s => s.snapshot!.trades)
  return <Panel title="Trade tape" eyebrow="EXECUTIONS" className="tape-panel"><div className="table-head"><span>PRICE</span><span>QTY</span><span>SIDE</span></div><div className="tape">{[...trades].reverse().slice(0, 18).map(trade => <div key={trade.id}><strong className={trade.aggressorSide === 'BUY' ? 'buy' : 'sell'}>{usd(trade.price)}</strong><span>{trade.quantity}</span><small>{trade.aggressorSide}</small></div>)}{!trades.length && <div className="empty">Waiting for the first execution</div>}</div></Panel>
}

function OrderTicket() {
  const snapshot = useSessionStore(s => s.snapshot!)
  const pending = useSessionStore(s => s.pending)
  const send = useSessionStore(s => s.send)
  const [side, setSide] = useState<Side>('BUY')
  const [type, setType] = useState<OrderType>('LIMIT')
  const [quantity, setQuantity] = useState(25)
  const [price, setPrice] = useState(snapshot.market.lastPrice / 100)
  const submit = (event: FormEvent) => {
    event.preventDefault()
    void send({ type: 'order.place', side, orderType: type, quantity, ...(type === 'LIMIT' ? { price: Math.round(price * 100) } : {}) }, `${side === 'BUY' ? 'Buy' : 'Sell'} order processed`)
  }
  const estimate = type === 'LIMIT' ? Math.round(price * 100) * quantity : snapshot.market.lastPrice * quantity
  return <Panel title="Manual order" eyebrow="NOVA TICKET" className="ticket-panel"><form onSubmit={submit}>
    <div className="segmented"><button type="button" className={side === 'BUY' ? 'active buy-tab' : ''} onClick={() => setSide('BUY')}>BUY</button><button type="button" className={side === 'SELL' ? 'active sell-tab' : ''} onClick={() => setSide('SELL')}>SELL</button></div>
    <fieldset disabled={snapshot.mode === 'REPLAY' || pending}><label><span>ORDER TYPE <i className="info-tip" title="Market orders execute only against available liquidity, never rest, and may be partially filled."><Info /></i></span><select value={type} onChange={event => setType(event.target.value as OrderType)}><option value="LIMIT">Limit</option><option value="MARKET">Market</option></select></label><label><span>QUANTITY</span><input type="number" min="1" max="1000000" value={quantity} onChange={event => setQuantity(Number(event.target.value))} required /></label>{type === 'LIMIT' && <label><span>LIMIT PRICE</span><div className="money-input"><i>$</i><input type="number" min="0.01" max="10000000" step="0.01" value={price} onChange={event => setPrice(Number(event.target.value))} required /></div></label>}</fieldset>
    <dl className="estimate"><div><dt>Estimated notional</dt><dd>{usd(estimate)}</dd></div><div><dt>Available</dt><dd>{side === 'BUY' ? usd(snapshot.account.cashAvailable) : `${integer(snapshot.account.sharesAvailable)} shares`}</dd></div></dl>
    <button className={`submit ${side.toLowerCase()}`} disabled={snapshot.mode === 'REPLAY' || pending || quantity < 1}>{snapshot.mode === 'REPLAY' ? 'READ-ONLY IN REPLAY' : `${side} ${quantity || 0} NOVA`}</button>
  </form></Panel>
}

function Portfolio() {
  const account = useSessionStore(s => s.snapshot!.account)
  const send = useSessionStore(s => s.send)
  const [tab, setTab] = useState<'orders' | 'fills'>('orders')
  return <Panel title="Portfolio & orders" eyebrow="SIMULATED FUNDS" className="portfolio-panel" action={<div className="mini-tabs"><button className={tab === 'orders' ? 'active' : ''} onClick={() => setTab('orders')}>OPEN {account.openOrders.length}</button><button className={tab === 'fills' ? 'active' : ''} onClick={() => setTab('fills')}>FILLS {account.tradeHistory.length}</button></div>}>
    <div className="account-cards"><div><span>EQUITY</span><strong>{usd(account.equity)}</strong><small>Cash + reserved + NOVA at mark</small></div><div><span>P&amp;L</span><strong className={account.pnl >= 0 ? 'buy' : 'sell'}>{usd(account.pnl, true)}</strong><small>Versus initial marked endowment</small></div><div><span>CASH / RESERVED</span><strong>{usd(account.cashAvailable)}</strong><small>{usd(account.cashReserved)} reserved</small></div><div><span>NOVA / RESERVED</span><strong>{integer(account.sharesAvailable)}</strong><small>{integer(account.sharesReserved)} reserved · mark: last trade</small></div></div>
    {tab === 'orders' ? <div className="data-table orders"><div className="table-head"><span>ID / SIDE</span><span>TYPE</span><span>PRICE</span><span>FILLED</span><span>STATUS</span><span /></div>{account.openOrders.map(order => <div className="table-row" key={order.id}><span><b>#{order.id}</b><small className={order.side === 'BUY' ? 'buy' : 'sell'}>{order.side}</small></span><span>{order.type}</span><span>{order.type === 'MARKET' ? 'MKT' : usd(order.price)}</span><span>{order.filled}/{order.quantity}</span><span>{order.status.replace('_', ' ')}</span><button onClick={() => void send({ type: 'order.cancel', orderId: order.id }, `Order #${order.id} canceled`)}>CANCEL</button></div>)}{!account.openOrders.length && <div className="empty">No open manual orders</div>}</div> : <div className="data-table fills"><div className="table-head"><span>TRADE</span><span>PRICE</span><span>QTY</span><span>ROLE</span></div>{[...account.tradeHistory].reverse().map(trade => <div className="table-row" key={trade.id}><span>#{trade.id}</span><span>{usd(trade.price)}</span><span>{trade.quantity}</span><span className={trade.buyer === 'you' ? 'buy' : 'sell'}>{trade.buyer === 'you' ? 'BOUGHT' : 'SOLD'}</span></div>)}{!account.tradeHistory.length && <div className="empty">Your fills will appear here</div>}</div>}
  </Panel>
}

function BotRow({ bot }: { bot: BotModel }) {
  return <div className="bot-row"><span className="bot-id"><i><Bot /></i><b>{bot.id}</b><small>{bot.strategy}</small></span><span><b>{integer(bot.inventory)}</b><small>inventory</small></span><span><b>{usd(bot.cash)}</b><small>cash</small></span><span className={bot.pnl >= 0 ? 'buy' : 'sell'}><b>{usd(bot.pnl, true)}</b><small>P&amp;L</small></span><span><em>{bot.status}</em><small>{bot.lastAction}</small></span></div>
}

function BotsPanel() {
  const snapshot = useSessionStore(s => s.snapshot!)
  const send = useSessionStore(s => s.send)
  const strategies = Object.entries(snapshot.strategies)
  const updateParam = (name: string, field: 'size' | 'interval' | 'spread' | 'threshold', value: number) => {
    const params = snapshot.strategies[name]
    void send({ type: 'strategy.update', strategy: name, enabled: params.enabled, params: { ...params, [field]: value } }, `${name} parameters updated`)
  }
  return <Panel title="Bot activity" eyebrow="DETERMINISTIC STRATEGIES" className="bots-panel">
    <div className="strategy-controls">{strategies.slice(0, 3).map(([name, params]) => <section key={name}>
      <header><strong>{name}</strong><label className="strategy-switch"><input type="checkbox" aria-label={`${name} enabled`} checked={params.enabled} disabled={snapshot.mode === 'REPLAY'} onChange={event => void send({ type: 'strategy.update', strategy: name, enabled: event.target.checked, params }, `${name} ${event.target.checked ? 'enabled' : 'disabled'}`)} /><span /></label></header>
      <div><label><span>CLIP SIZE</span><input aria-label={`${name} clip size`} type="number" min="1" max="1000" value={params.size} disabled={snapshot.mode === 'REPLAY' || !params.enabled} onChange={event => updateParam(name, 'size', Number(event.target.value))} /></label><label><span>INTERVAL MS</span><input aria-label={`${name} interval`} type="number" min="250" max="60000" step="250" value={params.interval} disabled={snapshot.mode === 'REPLAY' || !params.enabled} onChange={event => updateParam(name, 'interval', Number(event.target.value))} /></label>{params.spread !== undefined && <label><span>SPREAD ¢</span><input aria-label={`${name} spread`} type="number" min="0" value={params.spread} disabled={snapshot.mode === 'REPLAY' || !params.enabled} onChange={event => updateParam(name, 'spread', Number(event.target.value))} /></label>}{params.threshold !== undefined && <label><span>THRESHOLD ¢</span><input aria-label={`${name} threshold`} type="number" min="0" value={params.threshold} disabled={snapshot.mode === 'REPLAY' || !params.enabled} onChange={event => updateParam(name, 'threshold', Number(event.target.value))} /></label>}</div>
    </section>)}</div>
    {snapshot.mode === 'REPLAY' && <div className="readonly-note">Strategy controls are read-only during replay.</div>}
    <div className="bot-head"><span>PARTICIPANT</span><span>POSITION</span><span>CASH</span><span>P&amp;L</span><span>RECENT DECISION</span></div><div className="bot-table">{snapshot.bots.map(bot => <BotRow bot={bot} key={bot.id} />)}</div>
  </Panel>
}

const scenarios = [
  { id: 'negative-news', title: 'Negative news', copy: 'Lowers bot fair values, widens maker spreads, and submits a funded sell.', icon: <AlertTriangle /> },
  { id: 'liquidity-drought', title: 'Liquidity drought', copy: 'Cancels maker quotes and pauses their participation for 20 seconds.', icon: <Gauge /> },
  { id: 'large-sell', title: 'Large sell order', copy: 'Sends a real 600-share market sell into available bids.', icon: <Zap /> },
]

function EventInspector({ event, close }: { event: ScenarioEvent; close: () => void }) {
  return <div className="inspector" role="dialog" aria-modal="true" aria-labelledby="event-title"><button className="close" onClick={close} aria-label="Close event inspector"><X /></button><span className="eyebrow">EVENT #{event.sequence || '—'} · T+{simTime(event.logicalTime)}</span><h3 id="event-title">{event.title}</h3><p>{event.explanation}</p><div className="link-grid"><div><span>LINKED ORDERS</span><strong>{event.orderIds?.length ? event.orderIds.map(id => `#${id}`).join(', ') : 'None'}</strong></div><div><span>LINKED TRADES</span><strong>{event.tradeIds?.length ? event.tradeIds.map(id => `#${id}`).join(', ') : 'None'}</strong></div></div><small>Causal language describes simulator events only, not real-world predictions.</small></div>
}

function ScenariosPanel() {
  const snapshot = useSessionStore(s => s.snapshot!)
  const send = useSessionStore(s => s.send)
  const [selected, setSelected] = useState<ScenarioEvent | null>(null)
  return <Panel title="Scenarios & event inspector" eyebrow="CONTROLLED SHOCKS" className="scenarios-panel"><div className="scenario-grid">{scenarios.map(scenario => <button key={scenario.id} disabled={snapshot.mode === 'REPLAY'} onClick={() => void send({ type: 'scenario.load', scenarioId: scenario.id }, `${scenario.title} triggered`)}><i>{scenario.icon}</i><span><strong>{scenario.title}</strong><small>{scenario.copy}</small></span><ChevronRight /></button>)}</div><div className="events"><div className="event-head"><span>LOGICAL TIME</span><span>EVENT</span><span>LINKS</span></div>{[...snapshot.events].reverse().map((event, index) => <button key={`${event.type}-${event.logicalTime}-${index}`} onClick={() => setSelected(event)}><span>{simTime(event.logicalTime)}</span><span><i />{event.title}</span><span>{(event.orderIds?.length ?? 0)}O · {(event.tradeIds?.length ?? 0)}T</span><ChevronRight /></button>)}{!snapshot.events.length && <div className="empty">Trigger a scenario to create inspectable markers</div>}</div>{selected && <EventInspector event={selected} close={() => setSelected(null)} />}</Panel>
}

function ReplayPanel() {
  const snapshot = useSessionStore(s => s.snapshot!)
  const pending = useSessionStore(s => s.pending)
  const send = useSessionStore(s => s.send)
  const importExperiment = useSessionStore(s => s.importExperiment)
  const input = useRef<HTMLInputElement>(null)
  const replay = snapshot.mode === 'REPLAY'
  return <Panel title="Recording & replay" eyebrow="DETERMINISTIC EVENT STREAM" className="replay-panel" action={<span className={`mode-badge ${replay ? 'replay' : ''}`}>{snapshot.mode}</span>}><div className="replay-summary"><div><strong>{integer(snapshot.recording.eventCount)}</strong><span>RECORDED EVENTS</span></div><p>Commands use logical timestamps and sequence numbers. Replays apply recorded bot commands; bots are not regenerated.</p><small>Limit: {integer(snapshot.recording.maxEvents)} events · 5 MB imports</small></div>{replay ? <div className="timeline"><div className="timeline-controls"><button className="icon-button primary" onClick={() => void send({ type: snapshot.running ? 'replay.pause' : 'replay.play' })}>{snapshot.running ? <Pause /> : <Play />}</button><button className="icon-button" disabled={snapshot.running || snapshot.recording.atEnd} onClick={() => void send({ type: 'replay.step' }, 'Advanced one recorded event')}><StepForward /></button><strong>{snapshot.recording.replayIndex} / {snapshot.recording.eventCount}</strong></div><input aria-label="Replay event position" type="range" min="0" max={snapshot.recording.eventCount} value={snapshot.recording.replayIndex} onChange={event => void send({ type: 'replay.seek', index: Number(event.target.value) })} /></div> : <button className="enter-replay" disabled={pending || snapshot.recording.eventCount === 0} onClick={() => void send({ type: 'replay.enter' }, 'Entered read-only replay')}>PAUSE &amp; REPLAY RECORDING</button>}<div className="file-actions"><a className="secondary-button" href={sessionApi.exportUrl(snapshot.sessionId)} download={`marketlab-seed-${snapshot.seed}.json`}><Download /> DOWNLOAD JSON</a><button className="secondary-button" onClick={() => input.current?.click()}><Upload /> IMPORT JSON</button><input hidden ref={input} type="file" accept="application/json,.json" onChange={event => { const file = event.target.files?.[0]; if (file) void importExperiment(file) }} /></div></Panel>
}

function App() {
  const snapshot = useSessionStore(s => s.snapshot)
  const connection = useSessionStore(s => s.connection)
  const notice = useSessionStore(s => s.notice)
  const initialize = useSessionStore(s => s.initialize)
  const clearNotice = useSessionStore(s => s.clearNotice)
  useEffect(() => initialize(), [initialize])
  useEffect(() => { if (!notice) return; const timer = setTimeout(clearNotice, 4500); return () => clearTimeout(timer) }, [notice, clearNotice])
  const chartEvents = useMemo(() => snapshot?.events ?? [], [snapshot?.events])
  if (!snapshot) return <><Loading />{connection === 'disconnected' && <div className="fatal"><CloudOff /> Could not reach the MarketLab server.</div>}</>
  return <div className="app-shell">
    <header className="topbar"><div className="brand"><div className="logo">M<span>L</span></div><div><strong>MarketLab</strong><small>INTERACTIVE EXCHANGE</small></div></div><div className="symbol"><span>NOVA</span><small>FICTIONAL ASSET · USD</small></div><div className={`connection ${connection}`}><i />{connection.toUpperCase()}</div><div className={`mode ${snapshot.mode.toLowerCase()}`}>{snapshot.mode}</div></header>
    {connection !== 'connected' && <div className="connection-banner"><AlertTriangle /> {connection === 'reconnecting' ? 'Feed interrupted. Reconnecting for an authoritative snapshot…' : 'Disconnected. Controls are unavailable until the server returns.'}</div>}
    <SimulationControls /><MarketStrip />
    <main className="terminal">
      <Panel title="NOVA price & executed volume" eyebrow="5-SECOND LOGICAL BARS" className="chart-panel" action={<span className="chart-meta"><Activity /> {snapshot.candles.length} BARS</span>}>{snapshot.candles.length ? <MarketChart candles={snapshot.candles} events={chartEvents} /> : <div className="chart-empty"><Activity /><strong>Waiting for executions</strong><span>The chart is built only from backend trades.</span></div>}</Panel>
      <div className="right-stack"><MarketDepth /><TradeTape /></div>
      <OrderTicket /><Portfolio />
      <BotsPanel /><ScenariosPanel /><ReplayPanel />
    </main>
    <footer><span><CircleDollarSign /> All assets and funds are simulated. No order reaches a real market.</span><a href="https://www.tradingview.com/lightweight-charts/" target="_blank" rel="noreferrer">Charts by TradingView Lightweight Charts™ · Apache 2.0</a></footer>
    {notice && <div className={`toast ${notice.kind}`} role="status"><i>{notice.kind === 'success' ? '✓' : '!'}</i><span>{notice.message}</span><button onClick={clearNotice} aria-label="Dismiss"><X /></button></div>}
  </div>
}

export default App
