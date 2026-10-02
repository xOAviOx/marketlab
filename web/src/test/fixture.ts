import type { Snapshot } from '../types'

export const snapshot: Snapshot = {
  sessionId: 'test-session',
  seq: 7,
  mode: 'LIVE',
  running: false,
  speed: 1,
  logicalTime: 5_000,
  seed: 424242,
  symbol: 'NOVA',
  market: {
    lastPrice: 10_025,
    referencePrice: 10_000,
    runChange: 25,
    bestBid: 10_020,
    bestAsk: 10_030,
    spread: 10,
    volume: 120,
    markSource: 'Last executed trade',
  },
  book: {
    bids: [{ price: 10_020, quantity: 40, orders: 2 }],
    asks: [{ price: 10_030, quantity: 35, orders: 1 }],
  },
  trades: [{ id: 1, buyOrderId: 2, sellOrderId: 1, buyer: 'maker-alpha', seller: 'maker-beta', price: 10_025, quantity: 10, aggressorSide: 'BUY', sequence: 3 }],
  candles: [],
  account: {
    participant: 'you', cashAvailable: 10_000_000, cashReserved: 0,
    sharesAvailable: 1_000, sharesReserved: 0, equity: 20_025_000, pnl: 25_000,
    openOrders: [], tradeHistory: [],
  },
  bots: [{
    id: 'maker-alpha', strategy: 'Market maker', status: 'Quoting',
    lastAction: 'Refreshed two-sided quotes.', inventory: 5_000,
    cash: 20_000_000, equity: 70_125_000, pnl: 125_000,
    params: { enabled: true, spread: 16, size: 30, interval: 1_000, riskLimit: 1_500 },
  }],
  events: [{ sequence: 4, logicalTime: 5_000, type: 'negative-news', title: 'Negative news', explanation: 'Simulator event.', orderIds: [8], tradeIds: [3] }],
  recording: { eventCount: 12, replayIndex: 0, maxEvents: 50_000, atEnd: false },
  strategies: { 'Market maker': { enabled: true, spread: 16, size: 30, interval: 1_000, riskLimit: 1_500 } },
}

