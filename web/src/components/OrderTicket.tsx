import { useEffect, useMemo, useState } from 'react'
import type { OrderType, Side, TimeInForce } from '../types'
import { money } from '../lib/utils'
import { useSessionStore } from '../store/session'
import { Button, Panel } from './ui'

export function OrderTicket() {
  const instrument = useSessionStore(s => s.snapshot?.instruments.find(i => i.symbol === s.snapshot?.activeSymbol))
  const pending = useSessionStore(s => s.pending)
  const send = useSessionStore(s => s.send)
  const [side, setSide] = useState<Side>('buy')
  const [type, setType] = useState<OrderType>('limit')
  const [quantity, setQuantity] = useState('100')
  const [price, setPrice] = useState('')
  const [tif, setTif] = useState<TimeInForce>('GTC')
  useEffect(() => { if (instrument) setPrice(instrument.price.toFixed(2)) }, [instrument?.symbol])
  const estimated = useMemo(() => Number(quantity) * (type === 'market' ? instrument?.price ?? 0 : Number(price)), [quantity, price, type, instrument?.price])
  const valid = !!instrument && Number(quantity) > 0 && Number.isInteger(Number(quantity)) && (type === 'market' || Number(price) > 0)
  if (!instrument) return null
  return <Panel title="Order Ticket" aside={<span className="symbol-chip">{instrument.symbol}</span>}>
    <div className="ticket">
      <div className="segmented side-select"><button className={side === 'buy' ? 'buy-active' : ''} onClick={() => setSide('buy')}>BUY</button><button className={side === 'sell' ? 'sell-active' : ''} onClick={() => setSide('sell')}>SELL</button></div>
      <div className="segmented"><button className={type === 'market' ? 'active' : ''} onClick={() => setType('market')}>Market</button><button className={type === 'limit' ? 'active' : ''} onClick={() => setType('limit')}>Limit</button></div>
      <label>Quantity <span>shares</span><input inputMode="numeric" min="1" step="1" type="number" value={quantity} onChange={e => setQuantity(e.target.value)} /></label>
      <div className="quick-size">{[25, 50, 100, 250].map(size => <button key={size} onClick={() => setQuantity(String(size))}>{size}</button>)}</div>
      {type === 'limit' && <label>Limit price <span>USD</span><input inputMode="decimal" min="0.01" step="0.01" type="number" value={price} onChange={e => setPrice(e.target.value)} /></label>}
      <label>Time in force<select value={tif} onChange={e => setTif(e.target.value as TimeInForce)}><option>GTC</option><option>IOC</option><option>FOK</option></select></label>
      <div className="ticket-summary"><span>Estimated notional</span><strong>${money(estimated)}</strong><span>NBBO</span><strong>{money(instrument.bid)} × {money(instrument.ask)}</strong></div>
      <Button variant={side === 'buy' ? 'primary' : 'danger'} disabled={!valid || pending} onClick={() => void send({ type: 'order.place', symbol: instrument.symbol, side, orderType: type, quantity: Number(quantity), price: type === 'limit' ? Number(price) : undefined, timeInForce: tif })}>{pending ? 'TRANSMITTING…' : `${side.toUpperCase()} ${quantity || '0'} ${instrument.symbol}`}</Button>
      {!valid && <p className="field-error" role="alert">Enter a valid whole quantity{type === 'limit' ? ' and limit price' : ''}.</p>}
    </div>
  </Panel>
}
