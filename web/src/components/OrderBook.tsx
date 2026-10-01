import type { BookLevel, Trade } from '../types'
import { compact, money, stamp } from '../lib/utils'
import { Empty, Panel } from './ui'

function Level({ level, side, max }: { level: BookLevel; side: 'bid' | 'ask'; max: number }) {
  return <div className="book-row">
    <span className={`book-depth ${side}`} style={{ width: `${Math.max(4, level.size / max * 100)}%` }} />
    <span className={side === 'bid' ? 'up' : 'down'}>{money(level.price)}</span><span>{compact(level.size)}</span><span>{level.orders}</span>
  </div>
}

export function OrderBook({ bids, asks }: { bids: BookLevel[]; asks: BookLevel[] }) {
  const max = Math.max(...bids.map(x => x.size), ...asks.map(x => x.size), 1)
  const spread = asks.length && bids.length ? asks.at(-1)!.price - bids[0].price : 0
  return <Panel title="Depth" aside={<span className="micro">L2 · spread {money(spread)}</span>}>
    <div className="book-head"><span>PRICE</span><span>SIZE</span><span>ORD</span></div>
    {!asks.length && !bids.length ? <Empty>No depth available</Empty> : <div className="book">
      {asks.map(level => <Level key={level.price} level={level} side="ask" max={max} />)}
      <div className="spread-row"><span>MID</span><strong>{money((asks.at(-1)?.price ?? 0 + (bids[0]?.price ?? 0)) / 2)}</strong><span>{money(spread)}</span></div>
      {bids.map(level => <Level key={level.price} level={level} side="bid" max={max} />)}
    </div>}
  </Panel>
}

export function TradeTape({ trades }: { trades: Trade[] }) {
  return <Panel title="Tape" aside={<span className="live-label"><i /> LIVE</span>}>
    <div className="tape-head"><span>TIME</span><span>PRICE</span><span>SIZE</span></div>
    <div className="tape-list">{trades.length ? trades.map(trade => <div className="tape-row" key={trade.id} title={trade.venue}><span>{stamp(trade.time)}</span><strong className={trade.side === 'buy' ? 'up' : 'down'}>{money(trade.price)}</strong><span>{compact(trade.size)}</span></div>) : <Empty>No trades yet</Empty>}</div>
  </Panel>
}
