import type { Candle, SessionSnapshot } from '../types'

const start = Math.floor(Date.now() / 1000) - 59 * 60
const candles = (base: number): Candle[] => Array.from({ length: 60 }, (_, i) => {
  const wave = Math.sin(i / 5) * base * .004 + i * base * .00012
  const open = base + wave + Math.sin(i * 1.7) * base * .001
  const close = open + Math.sin(i * .8) * base * .0018
  return { time: start + i * 60, open, high: Math.max(open, close) + base * .0015, low: Math.min(open, close) - base * .0013, close, volume: 600 + ((i * 347) % 2200) }
})

export const demoSnapshot: SessionSnapshot = {
  version: 1,
  sessionId: 'ml-demo-7f3a',
  sequence: 1842,
  serverTime: new Date().toISOString(),
  simulation: { state: 'running', speed: 1, elapsed: 936, duration: 1800, scenarioId: 'liquidity-crunch' },
  activeSymbol: 'NVDA',
  instruments: [
    { symbol: 'NVDA', name: 'NVIDIA', price: 141.78, change: 2.84, changePercent: 2.04, bid: 141.77, ask: 141.79, volume: 48281000 },
    { symbol: 'AAPL', name: 'Apple', price: 226.31, change: -1.12, changePercent: -.49, bid: 226.30, ask: 226.33, volume: 31905000 },
    { symbol: 'TSLA', name: 'Tesla', price: 248.94, change: 5.72, changePercent: 2.35, bid: 248.90, ask: 248.98, volume: 71244000 },
    { symbol: 'SPY', name: 'S&P 500 ETF', price: 579.12, change: 1.06, changePercent: .18, bid: 579.11, ask: 579.13, volume: 54337000 },
    { symbol: 'BTC-USD', name: 'Bitcoin', price: 68142.40, change: -914.32, changePercent: -1.32, bid: 68140.10, ask: 68145.80, volume: 28410 },
  ],
  candles: { NVDA: candles(139.1), AAPL: candles(227.4), TSLA: candles(243.2), SPY: candles(578.1), 'BTC-USD': candles(69000) },
  orderBook: {
    asks: Array.from({ length: 9 }, (_, i) => ({ price: 141.79 + i * .01, size: [420, 980, 221, 1550, 340, 710, 460, 1220, 285][i], orders: 2 + i * 2 })).reverse(),
    bids: Array.from({ length: 9 }, (_, i) => ({ price: 141.77 - i * .01, size: [860, 340, 1120, 590, 240, 1830, 710, 395, 940][i], orders: 3 + i })),
  },
  trades: Array.from({ length: 16 }, (_, i) => ({ id: `t-${i}`, time: new Date(Date.now() - i * 4100).toISOString(), price: 141.78 + ([0, -.01, .01, 0, .02, -.02][i % 6]), size: [100, 25, 440, 80, 200, 15][i % 6], side: i % 3 === 0 ? 'sell' : 'buy', venue: ['NASDAQ', 'ARCA', 'BATS'][i % 3] as string })),
  portfolio: {
    cash: 42184.20, equity: 103746.82, buyingPower: 84368.40, dayPnl: 1284.62, totalPnl: 3746.82,
    positions: [
      { symbol: 'NVDA', quantity: 280, averagePrice: 137.42, markPrice: 141.78, unrealizedPnl: 1220.80, realizedPnl: 340 },
      { symbol: 'AAPL', quantity: 80, averagePrice: 228.10, markPrice: 226.31, unrealizedPnl: -143.20, realizedPnl: 95 },
      { symbol: 'SPY', quantity: 7, averagePrice: 574.20, markPrice: 579.12, unrealizedPnl: 34.44, realizedPnl: 0 },
    ],
  },
  openOrders: [
    { id: 'ord-91A4', time: new Date(Date.now() - 82000).toISOString(), symbol: 'NVDA', side: 'buy', type: 'limit', quantity: 120, filled: 0, price: 141.22, status: 'open' },
    { id: 'ord-43BC', time: new Date(Date.now() - 121000).toISOString(), symbol: 'AAPL', side: 'sell', type: 'limit', quantity: 40, filled: 10, price: 227.04, status: 'open' },
  ],
  orderHistory: [
    { id: 'ord-110F', time: new Date(Date.now() - 240000).toISOString(), symbol: 'NVDA', side: 'buy', type: 'market', quantity: 100, filled: 100, price: 140.92, status: 'filled' },
    { id: 'ord-7C02', time: new Date(Date.now() - 390000).toISOString(), symbol: 'TSLA', side: 'sell', type: 'limit', quantity: 20, filled: 0, price: 250, status: 'cancelled' },
    { id: 'ord-2AA8', time: new Date(Date.now() - 520000).toISOString(), symbol: 'AAPL', side: 'buy', type: 'limit', quantity: 1000, filled: 0, price: 220, status: 'rejected', rejectReason: 'Insufficient buying power' },
  ],
  botActivity: [
    { id: 'b1', time: new Date(Date.now() - 9000).toISOString(), bot: 'MM-ALPHA', action: 'REPRICED', detail: 'Tightened NVDA spread to 2¢', tone: 'positive' },
    { id: 'b2', time: new Date(Date.now() - 24000).toISOString(), bot: 'MOMENTUM-7', action: 'SIGNAL', detail: 'Buy pressure crossed 68%', tone: 'positive' },
    { id: 'b3', time: new Date(Date.now() - 46000).toISOString(), bot: 'RISK-GUARD', action: 'ALERT', detail: 'Inventory skew approaching limit', tone: 'negative' },
    { id: 'b4', time: new Date(Date.now() - 66000).toISOString(), bot: 'VWAP-2', action: 'FILLED', detail: '100 NVDA @ 140.92', tone: 'neutral' },
  ],
  scenarios: [
    { id: 'liquidity-crunch', name: 'Liquidity Crunch', description: 'Navigate a rapid withdrawal of displayed liquidity around a volatile earnings print.', difficulty: 'advanced', durationSeconds: 1800, tags: ['order book', 'risk'] },
    { id: 'opening-auction', name: 'Opening Auction', description: 'Manage price discovery and imbalances into the opening cross.', difficulty: 'intermediate', durationSeconds: 1200, tags: ['auction', 'volatility'] },
    { id: 'market-basics', name: 'Market Basics', description: 'Learn spread, order types, and execution mechanics in a calm market.', difficulty: 'intro', durationSeconds: 900, tags: ['guided', 'orders'] },
  ],
  events: [
    { id: 'e1', time: 120, kind: 'news', title: 'Earnings beat', description: 'EPS clears consensus by 11%. Guidance remains unchanged.', impact: '+ volatility', triggered: true },
    { id: 'e2', time: 540, kind: 'liquidity', title: 'Liquidity withdrawal', description: 'Top-of-book depth falls below the 10th percentile.', impact: 'spread × 2.4', triggered: true },
    { id: 'e3', time: 1020, kind: 'volatility', title: 'Momentum ignition', description: 'Aggressive flow enters after the consolidation break.', impact: '+ buy flow', triggered: false },
    { id: 'e4', time: 1440, kind: 'halt', title: 'Volatility pause', description: 'Limit-up/limit-down bands narrow around the reference price.', impact: 'possible halt', triggered: false },
  ],
}
