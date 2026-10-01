package sim

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strings"

	"marketlab/internal/engine"
)

const (
	FormatVersion   = 1
	EngineVersion   = "1.0.0"
	InitialPrice    = int64(10_000)
	MaxEvents       = 50_000
	MaxImportBytes  = 5 << 20
	HumanParticipant = "you"
)

type Config struct {
	InitialPrice int64 `json:"initialPrice"`
	StepMillis   int64 `json:"stepMillis"`
	MaxEvents    int   `json:"maxEvents"`
}

func DefaultConfig() Config {
	return Config{InitialPrice: InitialPrice, StepMillis: 250, MaxEvents: MaxEvents}
}

type RecordedEvent struct {
	Sequence    uint64                `json:"sequence"`
	LogicalTime int64                 `json:"logicalTime"`
	Kind        string                `json:"kind"`
	Origin      string                `json:"origin"`
	Reason      string                `json:"reason,omitempty"`
	Request     *engine.SubmitRequest `json:"request,omitempty"`
	OrderID     uint64                `json:"orderId,omitempty"`
	Participant string                `json:"participant,omitempty"`
	Scenario    string                `json:"scenario,omitempty"`
	Duration    int64                 `json:"duration,omitempty"`
	OrderIDs    []uint64              `json:"orderIds,omitempty"`
	TradeIDs    []uint64              `json:"tradeIds,omitempty"`
	Error       string                `json:"error,omitempty"`
}

type Experiment struct {
	FormatVersion int                  `json:"formatVersion"`
	EngineVersion string               `json:"engineVersion"`
	Seed          int64                `json:"seed"`
	Config        Config               `json:"config"`
	Endowments    []engine.Endowment   `json:"endowments"`
	Events        []RecordedEvent      `json:"events"`
}

type StrategyParams struct {
	Enabled   bool  `json:"enabled"`
	Spread    int64 `json:"spread,omitempty"`
	Size      int64 `json:"size"`
	Interval  int64 `json:"interval"`
	Lookback  int   `json:"lookback,omitempty"`
	Threshold int64 `json:"threshold,omitempty"`
	RiskLimit int64 `json:"riskLimit,omitempty"`
}

type Bot struct {
	ID          string         `json:"id"`
	Strategy    string         `json:"strategy"`
	Status      string         `json:"status"`
	LastAction  string         `json:"lastAction"`
	Inventory   int64          `json:"inventory"`
	Cash        int64          `json:"cash"`
	Equity      int64          `json:"equity"`
	PnL         int64          `json:"pnl"`
	NextRun     int64          `json:"-"`
	Bias        int64          `json:"-"`
	Params      StrategyParams `json:"params"`
}

type ScenarioMarker struct {
	Sequence    uint64   `json:"sequence"`
	LogicalTime int64    `json:"logicalTime"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	OrderIDs    []uint64 `json:"orderIds,omitempty"`
	TradeIDs    []uint64 `json:"tradeIds,omitempty"`
}

type Candle struct {
	Time   int64 `json:"time"`
	Open   int64 `json:"open"`
	High   int64 `json:"high"`
	Low    int64 `json:"low"`
	Close  int64 `json:"close"`
	Volume int64 `json:"volume"`
}

type Scheduled struct {
	At       int64
	Scenario string
}

type MarketView struct {
	LastPrice     int64 `json:"lastPrice"`
	ReferencePrice int64 `json:"referencePrice"`
	RunChange     int64 `json:"runChange"`
	BestBid       int64 `json:"bestBid"`
	BestAsk       int64 `json:"bestAsk"`
	Spread        int64 `json:"spread"`
	Volume        int64 `json:"volume"`
	MarkSource    string `json:"markSource"`
}

type AccountView struct {
	engine.Account
	Equity      int64          `json:"equity"`
	PnL         int64          `json:"pnl"`
	OpenOrders  []engine.Order `json:"openOrders"`
	TradeHistory []engine.Trade `json:"tradeHistory"`
}

type RecordingView struct {
	EventCount int `json:"eventCount"`
	ReplayIndex int `json:"replayIndex"`
	MaxEvents int `json:"maxEvents"`
	AtEnd bool `json:"atEnd"`
}

type Snapshot struct {
	SessionID  string           `json:"sessionId"`
	Seq        uint64           `json:"seq"`
	Mode       string           `json:"mode"`
	Running    bool             `json:"running"`
	Speed      float64          `json:"speed"`
	LogicalTime int64           `json:"logicalTime"`
	Seed       int64            `json:"seed"`
	Symbol     string           `json:"symbol"`
	Market     MarketView       `json:"market"`
	Book       struct{ Bids, Asks []engine.Level } `json:"book"`
	Trades     []engine.Trade   `json:"trades"`
	Candles    []Candle         `json:"candles"`
	Account    AccountView      `json:"account"`
	Bots       []Bot            `json:"bots"`
	Events     []ScenarioMarker `json:"events"`
	Recording  RecordingView    `json:"recording"`
	Strategies map[string]StrategyParams `json:"strategies"`
	Message    string           `json:"message,omitempty"`
}

type Simulation struct {
	ID            string
	Seed          int64
	Config        Config
	Endowments    []engine.Endowment
	Engine        *engine.Engine
	RNG           *rand.Rand
	LogicalTime   int64
	Running       bool
	Speed         float64
	Mode          string
	EventSeq      uint64
	Events        []RecordedEvent
	ReplaySource  []RecordedEvent
	ReplayIndex   int
	Bots          []Bot
	Markers       []ScenarioMarker
	Candles       []Candle
	ProcessedTrades int
	FairShift     int64
	StressUntil   int64
	DroughtUntil  int64
	Scheduled     []Scheduled
	Message       string
}

func defaultEndowments() []engine.Endowment {
	return []engine.Endowment{
		{Participant: HumanParticipant, Cash: 10_000_000, Shares: 1_000},
		{Participant: "maker-alpha", Cash: 20_000_000, Shares: 5_000},
		{Participant: "maker-beta", Cash: 20_000_000, Shares: 5_000},
		{Participant: "momentum-one", Cash: 10_000_000, Shares: 2_000},
		{Participant: "momentum-two", Cash: 10_000_000, Shares: 2_000},
		{Participant: "reversion-one", Cash: 10_000_000, Shares: 2_000},
		{Participant: "reversion-two", Cash: 10_000_000, Shares: 2_000},
		{Participant: "scenario-fund", Cash: 10_000_000, Shares: 10_000},
	}
}

func New(id string, seed int64) (*Simulation, error) {
	s, err := newSimulation(id, seed, DefaultConfig(), defaultEndowments())
	if err != nil {
		return nil, err
	}
	s.bootstrap()
	return s, nil
}

func newSimulation(id string, seed int64, config Config, endowments []engine.Endowment) (*Simulation, error) {
	e, err := engine.New(endowments)
	if err != nil {
		return nil, err
	}
	s := &Simulation{ID: id, Seed: seed, Config: config, Endowments: append([]engine.Endowment(nil), endowments...), Engine: e, RNG: rand.New(rand.NewSource(seed)), Speed: 1, Mode: "LIVE"}
	s.Bots = []Bot{
		{ID: "maker-alpha", Strategy: "Market maker", Status: "Ready", Bias: 12, Params: StrategyParams{Enabled: true, Spread: 16, Size: 30, Interval: 1_000, RiskLimit: 1_500}},
		{ID: "maker-beta", Strategy: "Market maker", Status: "Ready", Bias: -12, Params: StrategyParams{Enabled: true, Spread: 16, Size: 30, Interval: 1_000, RiskLimit: 1_500}},
		{ID: "momentum-one", Strategy: "Momentum", Status: "Ready", Params: StrategyParams{Enabled: true, Size: 12, Interval: 1_500, Lookback: 4, Threshold: 4, RiskLimit: 800}},
		{ID: "momentum-two", Strategy: "Momentum", Status: "Ready", Params: StrategyParams{Enabled: true, Size: 8, Interval: 2_000, Lookback: 6, Threshold: 5, RiskLimit: 800}},
		{ID: "reversion-one", Strategy: "Mean reversion", Status: "Ready", Params: StrategyParams{Enabled: true, Size: 10, Interval: 1_750, Threshold: 8, RiskLimit: 800}},
		{ID: "reversion-two", Strategy: "Mean reversion", Status: "Ready", Params: StrategyParams{Enabled: true, Size: 8, Interval: 2_250, Threshold: 12, RiskLimit: 800}},
	}
	return s, nil
}

func (s *Simulation) bootstrap() {
	for i := range s.Bots {
		if s.Bots[i].Strategy == "Market maker" {
			s.runMarketMaker(&s.Bots[i])
		}
	}
	s.consumeTrades(s.LogicalTime)
}

func (s *Simulation) record(event RecordedEvent) {
	if s.Mode != "LIVE" || len(s.Events) >= s.Config.MaxEvents {
		return
	}
	s.EventSeq++
	event.Sequence = s.EventSeq
	event.LogicalTime = s.LogicalTime
	s.Events = append(s.Events, event)
}

func (s *Simulation) submit(origin, reason string, req engine.SubmitRequest) (engine.Result, error) {
	before := len(s.Engine.Snapshot().Trades)
	result, err := s.Engine.Submit(req)
	event := RecordedEvent{Kind: "ORDER", Origin: origin, Reason: reason, Request: &req, Participant: req.Participant}
	if err != nil {
		event.Error = err.Error()
	} else {
		event.OrderID = result.Order.ID
		for _, trade := range result.Trades {
			event.TradeIDs = append(event.TradeIDs, trade.ID)
		}
	}
	s.record(event)
	if err == nil && len(s.Engine.Snapshot().Trades) > before {
		s.consumeTrades(s.LogicalTime)
	}
	return result, err
}

func (s *Simulation) Cancel(origin, participant string, orderID uint64, reason string) error {
	_, err := s.Engine.Cancel(participant, orderID)
	event := RecordedEvent{Kind: "CANCEL", Origin: origin, Reason: reason, OrderID: orderID, Participant: participant}
	if err != nil {
		event.Error = err.Error()
	}
	s.record(event)
	return err
}

func (s *Simulation) SubmitManual(req engine.SubmitRequest) (engine.Result, error) {
	if s.Mode != "LIVE" {
		return engine.Result{}, errors.New("trading is read-only during replay")
	}
	req.Participant = HumanParticipant
	return s.submit("MANUAL", "Manual order submitted.", req)
}

func (s *Simulation) Step() {
	if s.Mode == "REPLAY" {
		s.StepReplay()
		return
	}
	s.LogicalTime += s.Config.StepMillis
	s.applyScheduled()
	for i := range s.Bots {
		bot := &s.Bots[i]
		if !bot.Params.Enabled || s.LogicalTime < bot.NextRun {
			continue
		}
		bot.NextRun = s.LogicalTime + bot.Params.Interval
		switch bot.Strategy {
		case "Market maker":
			s.runMarketMaker(bot)
		case "Momentum":
			s.runMomentum(bot)
		case "Mean reversion":
			s.runMeanReversion(bot)
		}
	}
	s.consumeTrades(s.LogicalTime)
}

func (s *Simulation) runMarketMaker(bot *Bot) {
	for _, order := range s.Engine.Snapshot().Orders {
		if order.Participant == bot.ID && (order.Status == engine.Open || order.Status == engine.Partially) {
			_ = s.Cancel("BOT", bot.ID, order.ID, "Canceled stale quote before refresh.")
		}
	}
	if s.LogicalTime < s.DroughtUntil {
		bot.Status = "Withdrew"
		bot.LastAction = "Canceled quotes: liquidity drought active."
		return
	}
	spread := bot.Params.Spread
	if s.LogicalTime < s.StressUntil {
		spread *= 3
	}
	a, _ := s.Engine.Account(bot.ID)
	inventorySkew := (a.SharesAvailable + a.SharesReserved - 5_000) / 80
	fair := s.Config.InitialPrice + s.FairShift + bot.Bias - inventorySkew
	jitter := int64(s.RNG.Intn(3) - 1)
	bid, ask := fair-spread/2+jitter, fair+spread/2+jitter
	size := bot.Params.Size
	if s.LogicalTime < s.StressUntil {
		size = max(5, size/3)
	}
	_, bidErr := s.submit("BOT", fmt.Sprintf("Quoted bid around fair value %s inventory skew.", signed(-inventorySkew)), engine.SubmitRequest{Participant: bot.ID, Side: engine.Buy, Type: engine.Limit, Price: bid, Quantity: size})
	_, askErr := s.submit("BOT", fmt.Sprintf("Quoted ask around fair value %s inventory skew.", signed(-inventorySkew)), engine.SubmitRequest{Participant: bot.ID, Side: engine.Sell, Type: engine.Limit, Price: ask, Quantity: size})
	if bidErr != nil || askErr != nil {
		bot.Status = "Risk capped"
		bot.LastAction = "Quote skipped: balance or inventory risk limit reached."
	} else {
		bot.Status = "Quoting"
		bot.LastAction = fmt.Sprintf("Refreshed %d-share quotes at $%.2f / $%.2f.", size, float64(bid)/100, float64(ask)/100)
	}
}

func signed(v int64) string {
	if v >= 0 {
		return fmt.Sprintf("+%d", v)
	}
	return fmt.Sprintf("%d", v)
}

func (s *Simulation) recentPrices() []int64 {
	trades := s.Engine.Snapshot().Trades
	prices := make([]int64, len(trades))
	for i, trade := range trades {
		prices[i] = trade.Price
	}
	return prices
}

func (s *Simulation) runMomentum(bot *Bot) {
	prices := s.recentPrices()
	if len(prices) < bot.Params.Lookback {
		bot.Status, bot.LastAction = "Watching", "No order: waiting for enough executed-price history."
		return
	}
	change := prices[len(prices)-1] - prices[len(prices)-bot.Params.Lookback]
	if abs(change) < bot.Params.Threshold {
		bot.Status, bot.LastAction = "Watching", fmt.Sprintf("No order: %d¢ trend is below %d¢ threshold.", change, bot.Params.Threshold)
		return
	}
	side := engine.Buy
	if change < 0 {
		side = engine.Sell
	}
	_, err := s.submit("BOT", fmt.Sprintf("Followed %d¢ executed-price trend over %d trades.", change, bot.Params.Lookback), engine.SubmitRequest{Participant: bot.ID, Side: side, Type: engine.Market, Quantity: bot.Params.Size})
	if err != nil {
		bot.Status, bot.LastAction = "Risk capped", "Order rejected: available balance risk limit."
	} else {
		bot.Status, bot.LastAction = "Active", fmt.Sprintf("Submitted %s market order after %d¢ trend.", strings.ToLower(string(side)), change)
	}
}

func (s *Simulation) runMeanReversion(bot *Bot) {
	last, _ := s.lastPrice()
	fair := s.Config.InitialPrice + s.FairShift
	deviation := last - fair
	if abs(deviation) < bot.Params.Threshold {
		bot.Status, bot.LastAction = "Watching", fmt.Sprintf("No order: price is within %d¢ fair-value band.", bot.Params.Threshold)
		return
	}
	side := engine.Sell
	if deviation < 0 {
		side = engine.Buy
	}
	_, err := s.submit("BOT", fmt.Sprintf("Traded against %d¢ deviation from estimated fair value.", deviation), engine.SubmitRequest{Participant: bot.ID, Side: side, Type: engine.Market, Quantity: bot.Params.Size})
	if err != nil {
		bot.Status, bot.LastAction = "Risk capped", "Order rejected: strategy risk limit reached."
	} else {
		bot.Status, bot.LastAction = "Active", fmt.Sprintf("Submitted %s order against %d¢ deviation.", strings.ToLower(string(side)), deviation)
	}
}

func (s *Simulation) applyScheduled() {
	remaining := s.Scheduled[:0]
	for _, item := range s.Scheduled {
		if item.At > s.LogicalTime {
			remaining = append(remaining, item)
			continue
		}
		s.applyScenarioReversal(item.Scenario, true)
	}
	s.Scheduled = remaining
}

func (s *Simulation) TriggerScenario(name string) error {
	if s.Mode != "LIVE" {
		return errors.New("scenarios are disabled during replay")
	}
	switch name {
	case "negative-news":
		duration := int64(30_000)
		s.FairShift -= 800
		s.StressUntil = max(s.StressUntil, s.LogicalTime+duration)
		s.Scheduled = append(s.Scheduled, Scheduled{At: s.LogicalTime + duration, Scenario: name})
		s.record(RecordedEvent{Kind: "SCENARIO", Origin: "MANUAL", Scenario: name, Duration: duration, Reason: "Simulator event: bot fair-value estimates fell $8.00 and stress spreads widened."})
		r, err := s.submit("SCENARIO", "Negative-news fund sold through available bids.", engine.SubmitRequest{Participant: "scenario-fund", Side: engine.Sell, Type: engine.Market, Quantity: 180})
		marker := ScenarioMarker{LogicalTime: s.LogicalTime, Type: name, Title: "Negative news", Explanation: "Simulator event: fair-value estimates fell, makers widened quotes, and a funded participant submitted an actual sell order."}
		if err == nil {
			marker.OrderIDs = []uint64{r.Order.ID}
			for _, trade := range r.Trades { marker.TradeIDs = append(marker.TradeIDs, trade.ID) }
		}
		s.Markers = append(s.Markers, marker)
	case "liquidity-drought":
		duration := int64(20_000)
		s.DroughtUntil = max(s.DroughtUntil, s.LogicalTime+duration)
		s.Scheduled = append(s.Scheduled, Scheduled{At: s.LogicalTime + duration, Scenario: name})
		s.record(RecordedEvent{Kind: "SCENARIO", Origin: "MANUAL", Scenario: name, Duration: duration, Reason: "Simulator event: market makers withdrew quotes for 20 seconds."})
		for i := range s.Bots { if s.Bots[i].Strategy == "Market maker" { s.runMarketMaker(&s.Bots[i]) } }
		s.Markers = append(s.Markers, ScenarioMarker{LogicalTime: s.LogicalTime, Type: name, Title: "Liquidity drought", Explanation: "Simulator event: market makers canceled quotes and temporarily stopped participating."})
	case "large-sell":
		s.record(RecordedEvent{Kind: "SCENARIO", Origin: "MANUAL", Scenario: name, Reason: "Simulator event: a funded participant submitted a 600-share market sell."})
		r, err := s.submit("SCENARIO", "Large funded sell executed against real resting liquidity.", engine.SubmitRequest{Participant: "scenario-fund", Side: engine.Sell, Type: engine.Market, Quantity: 600})
		marker := ScenarioMarker{LogicalTime: s.LogicalTime, Type: name, Title: "Large sell order", Explanation: "Simulator event: a funded participant submitted an actual market sell; any move came from book liquidity."}
		if err == nil { marker.OrderIDs = []uint64{r.Order.ID}; for _, trade := range r.Trades { marker.TradeIDs = append(marker.TradeIDs, trade.ID) } }
		s.Markers = append(s.Markers, marker)
	default:
		return fmt.Errorf("unknown scenario %q", name)
	}
	if len(s.Markers) > 200 { s.Markers = s.Markers[len(s.Markers)-200:] }
	return nil
}

func (s *Simulation) applyScenarioReversal(name string, record bool) {
	switch name {
	case "negative-news":
		s.FairShift += 800
		s.StressUntil = s.LogicalTime
	case "liquidity-drought":
		s.DroughtUntil = s.LogicalTime
	}
	if record { s.record(RecordedEvent{Kind: "SCENARIO_END", Origin: "SCHEDULER", Scenario: name, Reason: "Scheduled scenario duration ended."}) }
}

func (s *Simulation) consumeTrades(at int64) {
	trades := s.Engine.Snapshot().Trades
	for s.ProcessedTrades < len(trades) {
		trade := trades[s.ProcessedTrades]
		bucket := at / 5_000 * 5_000
		if len(s.Candles) == 0 || s.Candles[len(s.Candles)-1].Time != bucket {
			s.Candles = append(s.Candles, Candle{Time: bucket, Open: trade.Price, High: trade.Price, Low: trade.Price, Close: trade.Price, Volume: trade.Quantity})
		} else {
			c := &s.Candles[len(s.Candles)-1]
			c.High, c.Low, c.Close, c.Volume = max(c.High, trade.Price), min(c.Low, trade.Price), trade.Price, c.Volume+trade.Quantity
		}
		s.ProcessedTrades++
	}
	if len(s.Candles) > 500 { s.Candles = s.Candles[len(s.Candles)-500:] }
}

func (s *Simulation) SetStrategy(strategy string, enabled bool, params *StrategyParams) error {
	if s.Mode != "LIVE" { return errors.New("strategies are read-only during replay") }
	found := false
	for i := range s.Bots {
		if strings.EqualFold(s.Bots[i].Strategy, strategy) {
			found = true
			s.Bots[i].Params.Enabled = enabled
			if params != nil {
				if params.Size <= 0 || params.Size > 1_000 || params.Interval < 250 || params.Interval > 60_000 { return errors.New("strategy parameters out of bounds") }
				s.Bots[i].Params.Size, s.Bots[i].Params.Interval = params.Size, params.Interval
				if params.Spread > 0 { s.Bots[i].Params.Spread = params.Spread }
				if params.Threshold > 0 { s.Bots[i].Params.Threshold = params.Threshold }
			}
			if !enabled { s.Bots[i].Status, s.Bots[i].LastAction = "Disabled", "Strategy disabled by operator." }
		}
	}
	if !found { return fmt.Errorf("unknown strategy %q", strategy) }
	s.record(RecordedEvent{Kind: "STRATEGY", Origin: "MANUAL", Reason: fmt.Sprintf("%s enabled=%t", strategy, enabled)})
	return nil
}

func (s *Simulation) Export() Experiment {
	return Experiment{FormatVersion: FormatVersion, EngineVersion: EngineVersion, Seed: s.Seed, Config: s.Config, Endowments: append([]engine.Endowment(nil), s.Endowments...), Events: append([]RecordedEvent(nil), s.Events...)}
}

func Import(id string, r io.Reader) (*Simulation, error) {
	limited := io.LimitReader(r, MaxImportBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil { return nil, err }
	if len(b) > MaxImportBytes { return nil, fmt.Errorf("experiment exceeds %d byte limit", MaxImportBytes) }
	var exp Experiment
	if err := json.Unmarshal(b, &exp); err != nil { return nil, fmt.Errorf("invalid experiment JSON: %w", err) }
	if exp.FormatVersion != FormatVersion || exp.EngineVersion != EngineVersion { return nil, fmt.Errorf("unsupported experiment version %d/%s", exp.FormatVersion, exp.EngineVersion) }
	if len(exp.Events) > MaxEvents || exp.Config.InitialPrice <= 0 || exp.Config.InitialPrice > engine.MaxPrice || exp.Config.StepMillis < 10 || exp.Config.StepMillis > 60_000 { return nil, errors.New("experiment configuration or event count out of bounds") }
	if len(exp.Endowments) == 0 || len(exp.Endowments) > 100 { return nil, errors.New("invalid experiment endowments") }
	s, err := newSimulation(id, exp.Seed, exp.Config, exp.Endowments)
	if err != nil { return nil, err }
	s.ReplaySource = append([]RecordedEvent(nil), exp.Events...)
	s.Mode, s.Running = "REPLAY", false
	return s, nil
}

func (s *Simulation) EnterReplay() error {
	if len(s.Events) == 0 { return errors.New("nothing has been recorded yet") }
	s.ReplaySource = append([]RecordedEvent(nil), s.Events...)
	return s.SeekReplay(0)
}

func (s *Simulation) SeekReplay(index int) error {
	if index < 0 || index > len(s.ReplaySource) { return errors.New("replay index out of range") }
	e, err := engine.New(s.Endowments)
	if err != nil { return err }
	s.Engine, s.Mode, s.Running, s.LogicalTime, s.ReplayIndex = e, "REPLAY", false, 0, 0
	s.Candles, s.Markers, s.Scheduled, s.ProcessedTrades = nil, nil, nil, 0
	s.FairShift, s.StressUntil, s.DroughtUntil = 0, 0, 0
	for s.ReplayIndex < index { if err := s.StepReplay(); err != nil { return err } }
	return nil
}

func (s *Simulation) StepReplay() error {
	if s.Mode != "REPLAY" { return errors.New("not in replay mode") }
	if s.ReplayIndex >= len(s.ReplaySource) { s.Running = false; return nil }
	event := s.ReplaySource[s.ReplayIndex]
	s.LogicalTime = event.LogicalTime
	switch event.Kind {
	case "ORDER":
		if event.Request == nil { return errors.New("recorded order is missing request") }
		_, err := s.Engine.Submit(*event.Request)
		if event.Error == "" && err != nil { return fmt.Errorf("replay diverged at event %d: %w", event.Sequence, err) }
		s.consumeTrades(event.LogicalTime)
	case "CANCEL":
		_, err := s.Engine.Cancel(event.Participant, event.OrderID)
		if event.Error == "" && err != nil { return fmt.Errorf("replay diverged at event %d: %w", event.Sequence, err) }
	case "SCENARIO":
		switch event.Scenario {
		case "negative-news": s.FairShift -= 800; s.StressUntil = event.LogicalTime + event.Duration
		case "liquidity-drought": s.DroughtUntil = event.LogicalTime + event.Duration
		}
		s.Markers = append(s.Markers, ScenarioMarker{Sequence: event.Sequence, LogicalTime: event.LogicalTime, Type: event.Scenario, Title: strings.ReplaceAll(event.Scenario, "-", " "), Explanation: event.Reason})
	case "SCENARIO_END":
		s.applyScenarioReversal(event.Scenario, false)
	}
	s.ReplayIndex++
	return nil
}

func (s *Simulation) lastPrice() (int64, string) {
	trades := s.Engine.Snapshot().Trades
	if len(trades) == 0 { return s.Config.InitialPrice, "Initial reference (no trades yet)" }
	return trades[len(trades)-1].Price, "Last executed trade"
}

func (s *Simulation) Snapshot() Snapshot {
	es := s.Engine.Snapshot()
	last, source := s.lastPrice()
	view := Snapshot{SessionID: s.ID, Seq: es.Sequence, Mode: s.Mode, Running: s.Running, Speed: s.Speed, LogicalTime: s.LogicalTime, Seed: s.Seed, Symbol: engine.Symbol, Trades: es.Trades, Candles: append([]Candle(nil), s.Candles...), Events: append([]ScenarioMarker(nil), s.Markers...), Strategies: map[string]StrategyParams{}, Message: s.Message}
	view.Book.Bids, view.Book.Asks = es.Bids, es.Asks
	view.Market = MarketView{LastPrice: last, ReferencePrice: s.Config.InitialPrice, RunChange: last - s.Config.InitialPrice, Volume: 0, MarkSource: source}
	for _, t := range es.Trades { view.Market.Volume += t.Quantity }
	if len(es.Bids) > 0 { view.Market.BestBid = es.Bids[0].Price }
	if len(es.Asks) > 0 { view.Market.BestAsk = es.Asks[0].Price }
	if view.Market.BestBid > 0 && view.Market.BestAsk > 0 { view.Market.Spread = view.Market.BestAsk - view.Market.BestBid }
	for _, a := range es.Accounts {
		if a.Participant != HumanParticipant { continue }
		view.Account.Account = a
		view.Account.Equity = a.CashAvailable + a.CashReserved + (a.SharesAvailable+a.SharesReserved)*last
		view.Account.PnL = view.Account.Equity - 10_000_000 - 1_000*s.Config.InitialPrice
	}
	for _, o := range es.Orders { if o.Participant == HumanParticipant && (o.Status == engine.Open || o.Status == engine.Partially) { view.Account.OpenOrders = append(view.Account.OpenOrders, o) } }
	for _, t := range es.Trades { if t.Buyer == HumanParticipant || t.Seller == HumanParticipant { view.Account.TradeHistory = append(view.Account.TradeHistory, t) } }
	view.Bots = append([]Bot(nil), s.Bots...)
	for i := range view.Bots {
		a, _ := s.Engine.Account(view.Bots[i].ID)
		view.Bots[i].Inventory = a.SharesAvailable + a.SharesReserved
		view.Bots[i].Cash = a.CashAvailable + a.CashReserved
		view.Bots[i].Equity = view.Bots[i].Cash + view.Bots[i].Inventory*last
		initialCash, initialShares := int64(0), int64(0)
		for _, x := range s.Endowments { if x.Participant == view.Bots[i].ID { initialCash, initialShares = x.Cash, x.Shares } }
		view.Bots[i].PnL = view.Bots[i].Equity - initialCash - initialShares*s.Config.InitialPrice
		view.Strategies[view.Bots[i].Strategy] = view.Bots[i].Params
	}
	sort.Slice(view.Bots, func(i, j int) bool { return view.Bots[i].ID < view.Bots[j].ID })
	count := len(s.Events)
	if s.Mode == "REPLAY" { count = len(s.ReplaySource) }
	view.Recording = RecordingView{EventCount: count, ReplayIndex: s.ReplayIndex, MaxEvents: s.Config.MaxEvents, AtEnd: s.Mode == "REPLAY" && s.ReplayIndex >= count}
	if len(view.Trades) > 200 { view.Trades = view.Trades[len(view.Trades)-200:] }
	if len(view.Account.TradeHistory) > 200 { view.Account.TradeHistory = view.Account.TradeHistory[len(view.Account.TradeHistory)-200:] }
	return view
}

func abs(v int64) int64 { if v < 0 { return -v }; return v }
func min(a, b int64) int64 { if a < b { return a }; return b }
func max(a, b int64) int64 { if a > b { return a }; return b }
