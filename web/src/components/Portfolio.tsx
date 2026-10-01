import * as Tabs from '@radix-ui/react-tabs'
import { X } from 'lucide-react'
import { useSessionStore } from '../store/session'
import { money, signed, stamp } from '../lib/utils'
import { Button, Empty, Metric, Panel, Tip } from './ui'

export function Portfolio() {
  const portfolio = useSessionStore(s => s.snapshot!.portfolio)
  const openOrders = useSessionStore(s => s.snapshot!.openOrders)
  const history = useSessionStore(s => s.snapshot!.orderHistory)
  const send = useSessionStore(s => s.send)
  return <Panel title="Portfolio" aside={<span className={portfolio.dayPnl >= 0 ? 'up' : 'down'}>{signed(portfolio.dayPnl)}</span>}>
    <div className="portfolio-metrics"><Metric label="EQUITY" value={`$${money(portfolio.equity)}`} /><Metric label="CASH" value={`$${money(portfolio.cash)}`} /><Metric label="BUYING POWER" value={`$${money(portfolio.buyingPower)}`} /></div>
    <Tabs.Root defaultValue="positions" className="data-tabs">
      <Tabs.List aria-label="Portfolio views"><Tabs.Trigger value="positions">Positions <b>{portfolio.positions.length}</b></Tabs.Trigger><Tabs.Trigger value="orders">Open <b>{openOrders.length}</b></Tabs.Trigger><Tabs.Trigger value="history">History</Tabs.Trigger></Tabs.List>
      <Tabs.Content value="positions"><div className="data-table"><div className="table-head"><span>SYMBOL</span><span>QTY</span><span>AVG / MARK</span><span>P&amp;L</span></div>{portfolio.positions.length ? portfolio.positions.map(p => <div className="table-row" key={p.symbol}><strong>{p.symbol}</strong><span>{p.quantity}</span><span>{money(p.averagePrice)} / {money(p.markPrice)}</span><strong className={p.unrealizedPnl >= 0 ? 'up' : 'down'}>{signed(p.unrealizedPnl)}</strong></div>) : <Empty>No positions</Empty>}</div></Tabs.Content>
      <Tabs.Content value="orders"><div className="data-table"><div className="table-head order-cols"><span>TIME</span><span>SIDE / SYMBOL</span><span>FILLED</span><span>PRICE</span><span /></div>{openOrders.length ? openOrders.map(o => <div className="table-row order-cols" key={o.id}><span>{stamp(o.time)}</span><strong className={o.side === 'buy' ? 'up' : 'down'}>{o.side.toUpperCase()} {o.symbol}</strong><span>{o.filled}/{o.quantity}</span><span>{o.price ? money(o.price) : 'MKT'}</span><Tip label="Cancel order"><Button aria-label={`Cancel order ${o.id}`} onClick={() => void send({ type: 'order.cancel', orderId: o.id })}><X size={12} /></Button></Tip></div>) : <Empty>No open orders</Empty>}</div></Tabs.Content>
      <Tabs.Content value="history"><div className="data-table"><div className="table-head"><span>TIME</span><span>ORDER</span><span>QTY</span><span>STATUS</span></div>{history.length ? history.map(o => <div className="table-row" key={o.id} title={o.rejectReason}><span>{stamp(o.time)}</span><strong>{o.side.toUpperCase()} {o.symbol}</strong><span>{o.quantity}</span><span className={`status-${o.status}`}>{o.status}</span></div>) : <Empty>No order history</Empty>}</div></Tabs.Content>
    </Tabs.Root>
  </Panel>
}
