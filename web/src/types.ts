export type Side = 'buy' | 'sell'
export type OrderType = 'market' | 'limit'
export type TimeInForce = 'GTC' | 'IOC' | 'FOK'
export type ConnectionState = 'loading' | 'connected' | 'reconnecting' | 'disconnected'
export type SimulationState = 'running' | 'paused' | 'complete'

export interface Instrument {
  symbol: string
  name: string
  price: number
  change: number
  changePercent: number
  bid: number
  ask: number
  volume: number
}

export interface Candle {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
}

export interface BookLevel { price: number; size: number; orders: number }
export interface Trade { id: string; time: string; price: number; size: number; side: Side; venue: string }
export interface Position { symbol: string; quantity: number; averagePrice: number; markPrice: number; unrealizedPnl: number; realizedPnl: number }
export interface Order { id: string; time: string; symbol: string; side: Side; type: OrderType; quantity: number; filled: number; price?: number; status: 'open' | 'filled' | 'cancelled' | 'rejected'; rejectReason?: string }
export interface Portfolio { cash: number; equity: number; buyingPower: number; dayPnl: number; totalPnl: number; positions: Position[] }
export interface BotActivity { id: string; time: string; bot: string; action: string; detail: string; tone: 'neutral' | 'positive' | 'negative' }
export interface ScenarioEvent { id: string; time: number; kind: 'news' | 'liquidity' | 'volatility' | 'halt'; title: string; description: string; impact: string; triggered: boolean }
export interface Scenario { id: string; name: string; description: string; difficulty: 'intro' | 'intermediate' | 'advanced'; durationSeconds: number; tags: string[] }

export interface SessionSnapshot {
  version: 1
  sessionId: string
  sequence: number
  serverTime: string
  simulation: { state: SimulationState; speed: number; elapsed: number; duration: number; scenarioId: string }
  instruments: Instrument[]
  activeSymbol: string
  candles: Record<string, Candle[]>
  orderBook: { bids: BookLevel[]; asks: BookLevel[] }
  trades: Trade[]
  portfolio: Portfolio
  openOrders: Order[]
  orderHistory: Order[]
  botActivity: BotActivity[]
  scenarios: Scenario[]
  events: ScenarioEvent[]
}

export type SessionCommand =
  | { type: 'simulation.pause' }
  | { type: 'simulation.resume' }
  | { type: 'simulation.restart' }
  | { type: 'simulation.seek'; elapsed: number }
  | { type: 'simulation.speed'; speed: number }
  | { type: 'instrument.select'; symbol: string }
  | { type: 'order.place'; symbol: string; side: Side; orderType: OrderType; quantity: number; price?: number; timeInForce: TimeInForce }
  | { type: 'order.cancel'; orderId: string }
  | { type: 'scenario.load'; scenarioId: string }

export type ServerMessage =
  | { type: 'snapshot'; snapshot: SessionSnapshot }
  | { type: 'patch'; sequence: number; snapshot: SessionSnapshot }
  | { type: 'command.accepted'; requestId: string }
  | { type: 'command.rejected'; requestId: string; reason: string }
  | { type: 'pong'; serverTime: string }

export interface CommandResponse { accepted: boolean; requestId: string; snapshot?: SessionSnapshot; reason?: string }
