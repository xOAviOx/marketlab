export type Side = 'BUY' | 'SELL'
export type OrderType = 'LIMIT' | 'MARKET'
export type ConnectionState = 'loading' | 'connected' | 'reconnecting' | 'disconnected'

export interface Level { price: number; quantity: number; orders: number }
export interface Trade { id: number; buyOrderId: number; sellOrderId: number; buyer: string; seller: string; price: number; quantity: number; aggressorSide: Side; sequence: number }
export interface Order { id: number; participant: string; side: Side; type: OrderType; price: number; quantity: number; filled: number; remaining: number; status: 'OPEN' | 'PARTIALLY_FILLED' | 'FILLED' | 'CANCELED'; sequence: number }
export interface Candle { time: number; open: number; high: number; low: number; close: number; volume: number }
export interface Market { lastPrice: number; referencePrice: number; runChange: number; bestBid: number; bestAsk: number; spread: number; volume: number; markSource: string }
export interface Account { participant: string; cashAvailable: number; cashReserved: number; sharesAvailable: number; sharesReserved: number; equity: number; pnl: number; openOrders: Order[]; tradeHistory: Trade[] }
export interface Bot { id: string; strategy: string; status: string; lastAction: string; inventory: number; cash: number; equity: number; pnl: number; params: StrategyParams }
export interface StrategyParams { enabled: boolean; spread?: number; size: number; interval: number; lookback?: number; threshold?: number; riskLimit?: number }
export interface ScenarioEvent { sequence: number; logicalTime: number; type: string; title: string; explanation: string; orderIds?: number[]; tradeIds?: number[] }
export interface Recording { eventCount: number; replayIndex: number; maxEvents: number; atEnd: boolean }

export interface Snapshot {
  sessionId: string
  seq: number
  mode: 'LIVE' | 'REPLAY'
  running: boolean
  speed: number
  logicalTime: number
  seed: number
  symbol: string
  market: Market
  book: { bids: Level[]; asks: Level[] }
  trades: Trade[]
  candles: Candle[]
  account: Account
  bots: Bot[]
  events: ScenarioEvent[]
  recording: Recording
  strategies: Record<string, StrategyParams>
  message?: string
}

export type Command =
  | { type: 'simulation.start' | 'simulation.resume' | 'simulation.pause' | 'simulation.step' }
  | { type: 'simulation.restart'; seed?: number }
  | { type: 'simulation.speed'; speed: number }
  | { type: 'order.place'; side: Side; orderType: OrderType; quantity: number; price?: number }
  | { type: 'order.cancel'; orderId: number }
  | { type: 'scenario.load'; scenarioId: string }
  | { type: 'replay.enter' | 'replay.step' | 'replay.play' | 'replay.pause' }
  | { type: 'replay.seek'; index: number }
  | { type: 'strategy.update'; strategy: string; enabled: boolean; params?: StrategyParams }

export interface CommandResponse { accepted: boolean; reason?: string; snapshot: Snapshot }
export type ServerMessage = { type: 'snapshot'; snapshot: Snapshot }
