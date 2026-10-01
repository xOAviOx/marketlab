package sim

import "marketlab/internal/engine"

const (
	FormatVersion = "marketlab.sim/v1"
	EngineVersion = "marketlab.engine/v1"

	HumanParticipant = "human"
	ShockParticipant = "shock-seller"

	MaxExportBytes = 5 << 20
	MaxCommands    = 50_000
	MaxEvents      = 50_000
)

type Mode string

const (
	ModeLive   Mode = "live"
	ModeReplay Mode = "replay"
)

type StrategyKind string

const (
	MarketMaker  StrategyKind = "market-maker"
	Momentum     StrategyKind = "momentum"
	MeanReverter StrategyKind = "mean-reversion"
)

type Scenario string

const (
	NegativeNews    Scenario = "negative-news"
	LiquidityDrought Scenario = "liquidity-drought"
	LargeSell       Scenario = "large-sell"
)

type CommandKind string

const (
	CommandTick     CommandKind = "tick"
	CommandSubmit   CommandKind = "submit"
	CommandCancel   CommandKind = "cancel"
	CommandStrategy CommandKind = "strategy"
	CommandScenario CommandKind = "scenario"
)

type EventKind string

const (
	EventOrder    EventKind = "order"
	EventTrade    EventKind = "trade"
	EventCancel   EventKind = "cancel"
	EventStrategy EventKind = "strategy"
	EventScenario EventKind = "scenario"
	EventRejected EventKind = "rejected"
	EventSystem   EventKind = "system"
)

type Reason struct {
	Code    string            `json:"code"`
	Summary string            `json:"summary"`
	Facts   map[string]string `json:"facts,omitempty"`
}

type Config struct {
	ReferencePrice       int64 `json:"referencePrice"`
	DefaultSpeed         int   `json:"defaultSpeed"`
	MaxTradeHistory      int   `json:"maxTradeHistory"`
	MaxOrderHistory      int   `json:"maxOrderHistory"`
	MaxCandleHistory     int   `json:"maxCandleHistory"`
	MaxEventHistory      int   `json:"maxEventHistory"`
	NegativeNewsDuration uint64 `json:"negativeNewsDuration"`
	DroughtDuration      uint64 `json:"droughtDuration"`
}

func DefaultConfig() Config {
	return Config{
		ReferencePrice:       10_000,
		DefaultSpeed:         2,
		MaxTradeHistory:      500,
		MaxOrderHistory:      500,
		MaxCandleHistory:     300,
		MaxEventHistory:      1_000,
		NegativeNewsDuration: 20,
		DroughtDuration:      20,
	}
}

type StrategyConfig struct {
	ID             string       `json:"id"`
	Participant    string       `json:"participant"`
	Kind           StrategyKind `json:"kind"`
	Enabled        bool         `json:"enabled"`
	Interval       uint64       `json:"interval"`
	OrderSize      int64        `json:"orderSize"`
	Spread         int64        `json:"spread,omitempty"`
	Threshold      int64        `json:"threshold,omitempty"`
	Lookback       int          `json:"lookback,omitempty"`
	Participation  int          `json:"participationBps"`
}

type StrategyUpdate struct {
	Enabled       *bool  `json:"enabled,omitempty"`
	Interval      uint64 `json:"interval,omitempty"`
	OrderSize     int64  `json:"orderSize,omitempty"`
	Spread        int64  `json:"spread,omitempty"`
	Threshold     int64  `json:"threshold,omitempty"`
	Lookback      int    `json:"lookback,omitempty"`
	Participation int    `json:"participationBps,omitempty"`
}

type CancelCommand struct {
	Participant string `json:"participant"`
	OrderID     uint64 `json:"orderId"`
}

type ScenarioCommand struct {
	Scenario         Scenario `json:"scenario"`
	Phase            string   `json:"phase"`
	EndTick          uint64   `json:"endTick,omitempty"`
	FairValueDelta   int64    `json:"fairValueDelta,omitempty"`
	ParticipationBps int      `json:"participationBps,omitempty"`
}

type Command struct {
	Version  int                   `json:"version"`
	Sequence uint64                `json:"sequence"`
	Tick     uint64                `json:"tick"`
	Kind     CommandKind           `json:"kind"`
	Source   string                `json:"source"`
	Reason   Reason                `json:"reason"`
	Submit   *engine.SubmitRequest `json:"submit,omitempty"`
	Cancel   *CancelCommand        `json:"cancel,omitempty"`
	Strategy *StrategyConfig       `json:"strategy,omitempty"`
	Scenario *ScenarioCommand      `json:"scenario,omitempty"`
}

type Event struct {
	Version   int               `json:"version"`
	Sequence  uint64            `json:"sequence"`
	Tick      uint64            `json:"tick"`
	Kind      EventKind         `json:"kind"`
	Source    string            `json:"source"`
	Message   string            `json:"message"`
	Reason    Reason            `json:"reason"`
	OrderID   uint64            `json:"orderId,omitempty"`
	TradeID   uint64            `json:"tradeId,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

type BookSnapshot struct {
	Bids []engine.Level `json:"bids"`
	Asks []engine.Level `json:"asks"`
}

type PriceChange struct {
	Absolute int64 `json:"absolute"`
	BasisPoints int64 `json:"basisPoints"`
}

type TradeSnapshot struct {
	engine.Trade
	Tick uint64 `json:"tick"`
}

type Candle struct {
	Tick   uint64 `json:"tick"`
	Open   int64  `json:"open"`
	High   int64  `json:"high"`
	Low    int64  `json:"low"`
	Close  int64  `json:"close"`
	Volume int64  `json:"volume"`
}

type Portfolio struct {
	CashAvailable   int64 `json:"cashAvailable"`
	CashReserved    int64 `json:"cashReserved"`
	SharesAvailable int64 `json:"sharesAvailable"`
	SharesReserved  int64 `json:"sharesReserved"`
	MarketValue     int64 `json:"marketValue"`
	Equity          int64 `json:"equity"`
	ProfitLoss      int64 `json:"profitLoss"`
}

type BotSnapshot struct {
	Config         StrategyConfig `json:"config"`
	OpenOrderIDs   []uint64       `json:"openOrderIds"`
	LastActionTick uint64         `json:"lastActionTick"`
}

type ReplayMetadata struct {
	FormatVersion string `json:"formatVersion"`
	EngineVersion string `json:"engineVersion"`
	Position      int    `json:"position"`
	Length        int    `json:"length"`
	AtEnd         bool   `json:"atEnd"`
}

type Snapshot struct {
	Session        string             `json:"session"`
	Mode           Mode               `json:"mode"`
	Running        bool               `json:"running"`
	Tick           uint64             `json:"tick"`
	Seed           int64              `json:"seed"`
	Speed          int                `json:"speed"`
	Reference      int64              `json:"reference"`
	Last           int64              `json:"last"`
	Change         PriceChange        `json:"change"`
	Spread         int64              `json:"spread"`
	Volume         int64              `json:"volume"`
	Book           BookSnapshot       `json:"book"`
	Trades         []TradeSnapshot    `json:"trades"`
	Candles        []Candle           `json:"candles"`
	Accounts       []engine.Account   `json:"accounts"`
	Human          Portfolio          `json:"human"`
	Orders         []engine.Order     `json:"orders"`
	Bots           []BotSnapshot      `json:"bots"`
	Events         []Event            `json:"events"`
	Replay         ReplayMetadata     `json:"replay"`
	Config         Config             `json:"config"`
}

type Recording struct {
	FormatVersion string    `json:"formatVersion"`
	EngineVersion string    `json:"engineVersion"`
	Seed          int64     `json:"seed"`
	Config        Config    `json:"config"`
	Commands      []Command `json:"commands"`
	Events        []Event   `json:"events"`
	FinalHash     string    `json:"finalHash"`
}
