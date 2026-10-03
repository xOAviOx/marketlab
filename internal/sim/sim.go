package sim

import (
	"crypto/sha256"
	"encoding/hex"
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
	FormatVersion    = 1
	EngineVersion    = "1.0.0"
	InitialPrice     = int64(10_000)
	MaxEvents        = 50_000
	MaxImportBytes   = 5 << 20
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
	Sequence        uint64                   `json:"sequence"`
	LogicalTime     int64                    `json:"logicalTime"`
	Kind            string                   `json:"kind"`
	Origin          string                   `json:"origin"`
	Reason          string                   `json:"reason,omitempty"`
	ReasonFacts     map[string]int64         `json:"reasonFacts,omitempty"`
	Request         *engine.SubmitRequest    `json:"request,omitempty"`
	OrderID         uint64                   `json:"orderId,omitempty"`
	Participant     string                   `json:"participant,omitempty"`
	Scenario        string                   `json:"scenario,omitempty"`
	Duration        int64                    `json:"duration,omitempty"`
	OrderIDs        []uint64                 `json:"orderIds,omitempty"`
	TradeIDs        []uint64                 `json:"tradeIds,omitempty"`
	StrategyChanges []RecordedStrategyChange `json:"strategyChanges,omitempty"`
	Error           string                   `json:"error,omitempty"`
}

type RecordedStrategyChange struct {
	BotID  string         `json:"botId"`
	Params StrategyParams `json:"params"`
}

type Experiment struct {
	FormatVersion int                `json:"formatVersion"`
	EngineVersion string             `json:"engineVersion"`
	Seed          int64              `json:"seed"`
	Config        Config             `json:"config"`
	Endowments    []engine.Endowment `json:"endowments"`
	Events        []RecordedEvent    `json:"events"`
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
	ID         string         `json:"id"`
	Strategy   string         `json:"strategy"`
	Status     string         `json:"status"`
	LastAction string         `json:"lastAction"`
	Inventory  int64          `json:"inventory"`
	Cash       int64          `json:"cash"`
	Equity     int64          `json:"equity"`
	PnL        int64          `json:"pnl"`
	NextRun    int64          `json:"-"`
	Bias       int64          `json:"-"`
	Params     StrategyParams `json:"params"`
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
	LastPrice      int64  `json:"lastPrice"`
	ReferencePrice int64  `json:"referencePrice"`
	RunChange      int64  `json:"runChange"`
	BestBid        int64  `json:"bestBid"`
	BestAsk        int64  `json:"bestAsk"`
	Spread         int64  `json:"spread"`
	Volume         int64  `json:"volume"`
	MarkSource     string `json:"markSource"`
}

type AccountView struct {
	engine.Account
	Equity       int64          `json:"equity"`
	PnL          int64          `json:"pnl"`
	OpenOrders   []engine.Order `json:"openOrders"`
	TradeHistory []engine.Trade `json:"tradeHistory"`
}

type RecordingView struct {
	EventCount  int  `json:"eventCount"`
	ReplayIndex int  `json:"replayIndex"`
	MaxEvents   int  `json:"maxEvents"`
	AtEnd       bool `json:"atEnd"`
}

type Snapshot struct {
	SessionID   string     `json:"sessionId"`
	Seq         uint64     `json:"seq"`
	Mode        string     `json:"mode"`
	Running     bool       `json:"running"`
	Speed       float64    `json:"speed"`
	LogicalTime int64      `json:"logicalTime"`
	Seed        int64      `json:"seed"`
	Symbol      string     `json:"symbol"`
	Market      MarketView `json:"market"`
	Book        struct {
		Bids []engine.Level `json:"bids"`
		Asks []engine.Level `json:"asks"`
	} `json:"book"`
	Trades     []engine.Trade            `json:"trades"`
	Candles    []Candle                  `json:"candles"`
	Accounts   []engine.Account          `json:"accounts"`
	Orders     []engine.Order            `json:"orders"`
	Account    AccountView               `json:"account"`
	Bots       []Bot                     `json:"bots"`
	Events     []ScenarioMarker          `json:"events"`
	Recording  RecordingView             `json:"recording"`
	Strategies map[string]StrategyParams `json:"strategies"`
	Config     Config                    `json:"config"`
	Message    string                    `json:"message,omitempty"`
}

type Simulation struct {
	ID              string
	Seed            int64
	Config          Config
	Endowments      []engine.Endowment
	Engine          *engine.Engine
	RNG             *rand.Rand
	LogicalTime     int64
	Running         bool
	Speed           float64
	Mode            string
	EventSeq        uint64
	Events          []RecordedEvent
	ReplaySource    []RecordedEvent
	ReplayIndex     int
	Bots            []Bot
	Markers         []ScenarioMarker
	Candles         []Candle
	ProcessedTrades int
	FairShift       int64
	StressUntil     int64
	DroughtUntil    int64
	Scheduled       []Scheduled
	Message         string
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
	return s.submitWithFacts(origin, reason, nil, req)
}

func (s *Simulation) submitWithFacts(origin, reason string, facts map[string]int64, req engine.SubmitRequest) (engine.Result, error) {
	before := len(s.Engine.Snapshot().Trades)
	result, err := s.Engine.Submit(req)
	event := RecordedEvent{Kind: "ORDER", Origin: origin, Reason: reason, ReasonFacts: facts, Request: &req, Participant: req.Participant}
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
	facts := map[string]int64{"fairValue": fair, "inventory": a.SharesAvailable + a.SharesReserved, "inventorySkew": inventorySkew, "spread": spread, "quantity": size}
	_, bidErr := s.submitWithFacts("BOT", fmt.Sprintf("Quoted bid around fair value %s inventory skew.", signed(-inventorySkew)), facts, engine.SubmitRequest{Participant: bot.ID, Side: engine.Buy, Type: engine.Limit, Price: bid, Quantity: size})
	_, askErr := s.submitWithFacts("BOT", fmt.Sprintf("Quoted ask around fair value %s inventory skew.", signed(-inventorySkew)), facts, engine.SubmitRequest{Participant: bot.ID, Side: engine.Sell, Type: engine.Limit, Price: ask, Quantity: size})
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
	size, inventory, lower, upper := s.riskBoundedSize(bot, side)
	if size == 0 {
		bot.Status = "Risk capped"
		bot.LastAction = fmt.Sprintf("No order: inventory %d is at the strategy range %d–%d.", inventory, lower, upper)
		return
	}
	reason := fmt.Sprintf("Followed %d¢ executed-price trend over %d trades; inventory %d, risk range %d–%d, submitted size %d.", change, bot.Params.Lookback, inventory, lower, upper, size)
	_, err := s.submitWithFacts("BOT", reason, map[string]int64{"priceChange": change, "lookback": int64(bot.Params.Lookback), "inventory": inventory, "riskLower": lower, "riskUpper": upper, "quantity": size}, engine.SubmitRequest{Participant: bot.ID, Side: side, Type: engine.Market, Quantity: size})
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
	size, inventory, lower, upper := s.riskBoundedSize(bot, side)
	if size == 0 {
		bot.Status = "Risk capped"
		bot.LastAction = fmt.Sprintf("No order: inventory %d is at the strategy range %d–%d.", inventory, lower, upper)
		return
	}
	reason := fmt.Sprintf("Traded against %d¢ deviation from estimated fair value; inventory %d, risk range %d–%d, submitted size %d.", deviation, inventory, lower, upper, size)
	_, err := s.submitWithFacts("BOT", reason, map[string]int64{"deviation": deviation, "fairValue": fair, "lastPrice": last, "inventory": inventory, "riskLower": lower, "riskUpper": upper, "quantity": size}, engine.SubmitRequest{Participant: bot.ID, Side: side, Type: engine.Market, Quantity: size})
	if err != nil {
		bot.Status, bot.LastAction = "Risk capped", "Order rejected: strategy risk limit reached."
	} else {
		bot.Status, bot.LastAction = "Active", fmt.Sprintf("Submitted %s order against %d¢ deviation.", strings.ToLower(string(side)), deviation)
	}
}

func (s *Simulation) riskBoundedSize(bot *Bot, side engine.Side) (size, inventory, lower, upper int64) {
	account, ok := s.Engine.Account(bot.ID)
	if !ok {
		return 0, 0, 0, 0
	}
	inventory = account.SharesAvailable + account.SharesReserved
	initial := inventory
	for _, endowment := range s.Endowments {
		if endowment.Participant == bot.ID {
			initial = endowment.Shares
			break
		}
	}
	lower = max(0, initial-bot.Params.RiskLimit)
	upper = initial + bot.Params.RiskLimit
	size = bot.Params.Size
	if bot.Params.RiskLimit <= 0 {
		return 0, inventory, initial, initial
	}
	if side == engine.Buy {
		size = min(size, max(0, upper-inventory))
	} else {
		size = min(size, max(0, inventory-lower))
	}
	return size, inventory, lower, upper
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
		sequence := s.EventSeq
		r, err := s.submit("SCENARIO", "Negative-news fund sold through available bids.", engine.SubmitRequest{Participant: "scenario-fund", Side: engine.Sell, Type: engine.Market, Quantity: 180})
		marker := ScenarioMarker{Sequence: sequence, LogicalTime: s.LogicalTime, Type: name, Title: "Negative news", Explanation: "Simulator event: fair-value estimates fell, makers widened quotes, and a funded participant submitted an actual sell order."}
		if err == nil {
			marker.OrderIDs = []uint64{r.Order.ID}
			for _, trade := range r.Trades {
				marker.TradeIDs = append(marker.TradeIDs, trade.ID)
			}
		}
		s.Markers = append(s.Markers, marker)
	case "liquidity-drought":
		duration := int64(20_000)
		s.DroughtUntil = max(s.DroughtUntil, s.LogicalTime+duration)
		s.Scheduled = append(s.Scheduled, Scheduled{At: s.LogicalTime + duration, Scenario: name})
		s.record(RecordedEvent{Kind: "SCENARIO", Origin: "MANUAL", Scenario: name, Duration: duration, Reason: "Simulator event: market makers withdrew quotes for 20 seconds."})
		sequence := s.EventSeq
		for i := range s.Bots {
			if s.Bots[i].Strategy == "Market maker" {
				s.runMarketMaker(&s.Bots[i])
			}
		}
		s.Markers = append(s.Markers, ScenarioMarker{Sequence: sequence, LogicalTime: s.LogicalTime, Type: name, Title: "Liquidity drought", Explanation: "Simulator event: market makers canceled quotes and temporarily stopped participating."})
	case "large-sell":
		s.record(RecordedEvent{Kind: "SCENARIO", Origin: "MANUAL", Scenario: name, Reason: "Simulator event: a funded participant submitted a 600-share market sell."})
		sequence := s.EventSeq
		r, err := s.submit("SCENARIO", "Large funded sell executed against real resting liquidity.", engine.SubmitRequest{Participant: "scenario-fund", Side: engine.Sell, Type: engine.Market, Quantity: 600})
		marker := ScenarioMarker{Sequence: sequence, LogicalTime: s.LogicalTime, Type: name, Title: "Large sell order", Explanation: "Simulator event: a funded participant submitted an actual market sell; any move came from book liquidity."}
		if err == nil {
			marker.OrderIDs = []uint64{r.Order.ID}
			for _, trade := range r.Trades {
				marker.TradeIDs = append(marker.TradeIDs, trade.ID)
			}
		}
		s.Markers = append(s.Markers, marker)
	default:
		return fmt.Errorf("unknown scenario %q", name)
	}
	if len(s.Markers) > 200 {
		s.Markers = s.Markers[len(s.Markers)-200:]
	}
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
	if record {
		s.record(RecordedEvent{Kind: "SCENARIO_END", Origin: "SCHEDULER", Scenario: name, Reason: "Scheduled scenario duration ended."})
	}
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
	if len(s.Candles) > 500 {
		s.Candles = s.Candles[len(s.Candles)-500:]
	}
}

func (s *Simulation) SetStrategy(strategy string, enabled bool, params *StrategyParams) error {
	if s.Mode != "LIVE" {
		return errors.New("strategies are read-only during replay")
	}
	if params != nil {
		if params.Size <= 0 || params.Size > 1_000 || params.Interval < 250 || params.Interval > 60_000 || params.Spread < 0 || params.Spread > engine.MaxPrice || params.Lookback < 0 || params.Lookback > 1_000 || params.Threshold < 0 || params.Threshold > engine.MaxPrice || params.RiskLimit < 0 || params.RiskLimit > engine.MaxQuantity {
			return errors.New("strategy parameters out of bounds")
		}
	}
	found := false
	var changes []RecordedStrategyChange
	for i := range s.Bots {
		if strings.EqualFold(s.Bots[i].Strategy, strategy) {
			found = true
			s.Bots[i].Params.Enabled = enabled
			if params != nil {
				s.Bots[i].Params.Size, s.Bots[i].Params.Interval = params.Size, params.Interval
				if params.Spread > 0 {
					s.Bots[i].Params.Spread = params.Spread
				}
				if params.Threshold > 0 {
					s.Bots[i].Params.Threshold = params.Threshold
				}
				if params.Lookback > 0 {
					s.Bots[i].Params.Lookback = params.Lookback
				}
				if params.RiskLimit > 0 {
					s.Bots[i].Params.RiskLimit = params.RiskLimit
				}
			}
			if !enabled {
				s.Bots[i].Status, s.Bots[i].LastAction = "Disabled", "Strategy disabled by operator."
			}
			changes = append(changes, RecordedStrategyChange{BotID: s.Bots[i].ID, Params: s.Bots[i].Params})
		}
	}
	if !found {
		return fmt.Errorf("unknown strategy %q", strategy)
	}
	s.record(RecordedEvent{Kind: "STRATEGY", Origin: "MANUAL", Reason: fmt.Sprintf("%s enabled=%t", strategy, enabled), StrategyChanges: changes})
	return nil
}

func (s *Simulation) Export() Experiment {
	return Experiment{FormatVersion: FormatVersion, EngineVersion: EngineVersion, Seed: s.Seed, Config: s.Config, Endowments: append([]engine.Endowment(nil), s.Endowments...), Events: append([]RecordedEvent(nil), s.Events...)}
}

func Import(id string, r io.Reader) (*Simulation, error) {
	limited := io.LimitReader(r, MaxImportBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxImportBytes {
		return nil, fmt.Errorf("experiment exceeds %d byte limit", MaxImportBytes)
	}
	var exp Experiment
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&exp); err != nil {
		return nil, fmt.Errorf("invalid experiment JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("experiment contains trailing JSON data")
	}
	if exp.FormatVersion != FormatVersion || exp.EngineVersion != EngineVersion {
		return nil, fmt.Errorf("unsupported experiment version %d/%s", exp.FormatVersion, exp.EngineVersion)
	}
	if len(exp.Events) > MaxEvents || exp.Config.InitialPrice <= 0 || exp.Config.InitialPrice > engine.MaxPrice || exp.Config.StepMillis < 10 || exp.Config.StepMillis > 60_000 || exp.Config.MaxEvents < 1 || exp.Config.MaxEvents > MaxEvents {
		return nil, errors.New("experiment configuration or event count out of bounds")
	}
	if len(exp.Endowments) == 0 || len(exp.Endowments) > 100 {
		return nil, errors.New("invalid experiment endowments")
	}
	for _, endowment := range exp.Endowments {
		if endowment.Participant == "" || len(endowment.Participant) > 128 || endowment.Cash < 0 || endowment.Cash > 1_000_000_000_000_000 || endowment.Shares < 0 || endowment.Shares > 1_000_000_000_000_000 {
			return nil, errors.New("experiment endowment field out of bounds")
		}
	}
	if err := validateRecordedEvents(exp.Events); err != nil {
		return nil, err
	}
	s, err := newSimulation(id, exp.Seed, exp.Config, exp.Endowments)
	if err != nil {
		return nil, err
	}
	s.ReplaySource = append([]RecordedEvent(nil), exp.Events...)
	s.Mode, s.Running = "REPLAY", false
	return s, nil
}

func validateRecordedEvents(events []RecordedEvent) error {
	var sequence uint64
	var logicalTime int64
	for i, event := range events {
		if event.Sequence <= sequence || event.Sequence > 10_000_000 || event.LogicalTime < logicalTime || event.LogicalTime > 1_000_000_000_000 {
			return fmt.Errorf("event %d sequence or logical time is not ordered or is out of bounds", i)
		}
		if len(event.Kind) > 32 || len(event.Origin) > 64 || len(event.Reason) > 2_048 || len(event.Error) > 2_048 || len(event.Participant) > 128 || len(event.Scenario) > 64 || len(event.ReasonFacts) > 32 || len(event.OrderIDs) > 1_000 || len(event.TradeIDs) > 1_000 {
			return fmt.Errorf("event %d contains an oversized field", i)
		}
		if event.Duration < 0 || event.Duration > 1_000_000_000 || event.OrderID > 10_000_000 {
			return fmt.Errorf("event %d numeric field is out of bounds", i)
		}
		switch event.Kind {
		case "ORDER":
			if event.Request == nil {
				return fmt.Errorf("event %d order is missing request", i)
			}
			request := event.Request
			if len(request.Participant) == 0 || len(request.Participant) > 128 || len(request.Side) > 16 || len(request.Type) > 16 || request.Price < -engine.MaxPrice || request.Price > engine.MaxPrice || request.Quantity < -engine.MaxQuantity || request.Quantity > engine.MaxQuantity {
				return fmt.Errorf("event %d order field is out of bounds", i)
			}
		case "CANCEL":
			if event.Participant == "" {
				return fmt.Errorf("event %d cancel is missing participant", i)
			}
		case "SCENARIO", "SCENARIO_END":
			if event.Scenario != "negative-news" && event.Scenario != "liquidity-drought" && event.Scenario != "large-sell" {
				return fmt.Errorf("event %d has unknown scenario %q", i, event.Scenario)
			}
		case "STRATEGY":
			if len(event.StrategyChanges) == 0 || len(event.StrategyChanges) > 100 {
				return fmt.Errorf("event %d strategy configuration is missing or oversized", i)
			}
			for _, change := range event.StrategyChanges {
				params := change.Params
				if change.BotID == "" || len(change.BotID) > 128 || params.Size < 1 || params.Size > 1_000 || params.Interval < 250 || params.Interval > 60_000 || params.Spread < 0 || params.Spread > engine.MaxPrice || params.Lookback < 0 || params.Lookback > 1_000 || params.Threshold < 0 || params.Threshold > engine.MaxPrice || params.RiskLimit < 0 || params.RiskLimit > engine.MaxQuantity {
					return fmt.Errorf("event %d strategy field is out of bounds", i)
				}
			}
		default:
			return fmt.Errorf("event %d has unknown kind %q", i, event.Kind)
		}
		sequence, logicalTime = event.Sequence, event.LogicalTime
	}
	return nil
}

func (s *Simulation) EnterReplay() error {
	if len(s.Events) == 0 {
		return errors.New("nothing has been recorded yet")
	}
	s.ReplaySource = append([]RecordedEvent(nil), s.Events...)
	return s.SeekReplay(0)
}

func (s *Simulation) SeekReplay(index int) error {
	if index < 0 || index > len(s.ReplaySource) {
		return errors.New("replay index out of range")
	}
	fresh, err := newSimulation(s.ID, s.Seed, s.Config, s.Endowments)
	if err != nil {
		return err
	}
	replaySource := append([]RecordedEvent(nil), s.ReplaySource...)
	fresh.ReplaySource = replaySource
	fresh.Mode = "REPLAY"
	*s = *fresh
	for s.ReplayIndex < index {
		if err := s.StepReplay(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Simulation) StepReplay() error {
	if s.Mode != "REPLAY" {
		return errors.New("not in replay mode")
	}
	if s.ReplayIndex >= len(s.ReplaySource) {
		s.Running = false
		return nil
	}
	event := s.ReplaySource[s.ReplayIndex]
	s.LogicalTime = event.LogicalTime
	switch event.Kind {
	case "ORDER":
		if event.Request == nil {
			return errors.New("recorded order is missing request")
		}
		_, err := s.Engine.Submit(*event.Request)
		if (event.Error == "") != (err == nil) {
			return fmt.Errorf("replay acceptance diverged at event %d: recorded error %q, replay error %v", event.Sequence, event.Error, err)
		}
		s.consumeTrades(event.LogicalTime)
	case "CANCEL":
		_, err := s.Engine.Cancel(event.Participant, event.OrderID)
		if (event.Error == "") != (err == nil) {
			return fmt.Errorf("replay acceptance diverged at event %d: recorded error %q, replay error %v", event.Sequence, event.Error, err)
		}
	case "SCENARIO":
		switch event.Scenario {
		case "negative-news":
			s.FairShift -= 800
			s.StressUntil = event.LogicalTime + event.Duration
		case "liquidity-drought":
			s.DroughtUntil = event.LogicalTime + event.Duration
		}
		s.Markers = append(s.Markers, ScenarioMarker{Sequence: event.Sequence, LogicalTime: event.LogicalTime, Type: event.Scenario, Title: strings.ReplaceAll(event.Scenario, "-", " "), Explanation: event.Reason})
	case "SCENARIO_END":
		s.applyScenarioReversal(event.Scenario, false)
	case "STRATEGY":
		if len(event.StrategyChanges) == 0 {
			return fmt.Errorf("recorded strategy event %d is missing configuration", event.Sequence)
		}
		for _, change := range event.StrategyChanges {
			found := false
			for i := range s.Bots {
				if s.Bots[i].ID == change.BotID {
					s.Bots[i].Params = change.Params
					if !change.Params.Enabled {
						s.Bots[i].Status, s.Bots[i].LastAction = "Disabled", "Strategy disabled by operator."
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("recorded strategy event %d references unknown bot %q", event.Sequence, change.BotID)
			}
		}
	}
	s.ReplayIndex++
	return nil
}

func (s *Simulation) lastPrice() (int64, string) {
	trades := s.Engine.Snapshot().Trades
	if len(trades) == 0 {
		return s.Config.InitialPrice, "Initial reference (no trades yet)"
	}
	return trades[len(trades)-1].Price, "Last executed trade"
}

func (s *Simulation) Snapshot() Snapshot {
	es := s.Engine.Snapshot()
	last, source := s.lastPrice()
	view := Snapshot{SessionID: s.ID, Seq: es.Sequence, Mode: s.Mode, Running: s.Running, Speed: s.Speed, LogicalTime: s.LogicalTime, Seed: s.Seed, Symbol: engine.Symbol, Trades: es.Trades, Candles: append([]Candle(nil), s.Candles...), Accounts: es.Accounts, Orders: es.Orders, Events: append([]ScenarioMarker(nil), s.Markers...), Strategies: map[string]StrategyParams{}, Config: s.Config, Message: s.Message}
	view.Book.Bids, view.Book.Asks = es.Bids, es.Asks
	view.Market = MarketView{LastPrice: last, ReferencePrice: s.Config.InitialPrice, RunChange: last - s.Config.InitialPrice, Volume: 0, MarkSource: source}
	for _, t := range es.Trades {
		view.Market.Volume += t.Quantity
	}
	if len(es.Bids) > 0 {
		view.Market.BestBid = es.Bids[0].Price
	}
	if len(es.Asks) > 0 {
		view.Market.BestAsk = es.Asks[0].Price
	}
	if view.Market.BestBid > 0 && view.Market.BestAsk > 0 {
		view.Market.Spread = view.Market.BestAsk - view.Market.BestBid
	}
	for _, a := range es.Accounts {
		if a.Participant != HumanParticipant {
			continue
		}
		view.Account.Account = a
		view.Account.Equity = a.CashAvailable + a.CashReserved + (a.SharesAvailable+a.SharesReserved)*last
		view.Account.PnL = view.Account.Equity - 10_000_000 - 1_000*s.Config.InitialPrice
	}
	for _, o := range es.Orders {
		if o.Participant == HumanParticipant && (o.Status == engine.Open || o.Status == engine.Partially) {
			view.Account.OpenOrders = append(view.Account.OpenOrders, o)
		}
	}
	for _, t := range es.Trades {
		if t.Buyer == HumanParticipant || t.Seller == HumanParticipant {
			view.Account.TradeHistory = append(view.Account.TradeHistory, t)
		}
	}
	view.Bots = append([]Bot(nil), s.Bots...)
	for i := range view.Bots {
		a, _ := s.Engine.Account(view.Bots[i].ID)
		view.Bots[i].Inventory = a.SharesAvailable + a.SharesReserved
		view.Bots[i].Cash = a.CashAvailable + a.CashReserved
		view.Bots[i].Equity = view.Bots[i].Cash + view.Bots[i].Inventory*last
		initialCash, initialShares := int64(0), int64(0)
		for _, x := range s.Endowments {
			if x.Participant == view.Bots[i].ID {
				initialCash, initialShares = x.Cash, x.Shares
			}
		}
		view.Bots[i].PnL = view.Bots[i].Equity - initialCash - initialShares*s.Config.InitialPrice
		view.Strategies[view.Bots[i].Strategy] = view.Bots[i].Params
	}
	sort.Slice(view.Bots, func(i, j int) bool { return view.Bots[i].ID < view.Bots[j].ID })
	count := len(s.Events)
	if s.Mode == "REPLAY" {
		count = len(s.ReplaySource)
	}
	view.Recording = RecordingView{EventCount: count, ReplayIndex: s.ReplayIndex, MaxEvents: s.Config.MaxEvents, AtEnd: s.Mode == "REPLAY" && s.ReplayIndex >= count}
	if len(view.Trades) > 200 {
		view.Trades = view.Trades[len(view.Trades)-200:]
	}
	if len(view.Account.TradeHistory) > 200 {
		view.Account.TradeHistory = view.Account.TradeHistory[len(view.Account.TradeHistory)-200:]
	}
	if len(view.Orders) > 500 {
		view.Orders = view.Orders[len(view.Orders)-500:]
	}
	// The browser contract uses arrays rather than nullable collections. Keeping
	// empty collections explicit also lets consumers safely render a new session.
	if view.Book.Bids == nil {
		view.Book.Bids = []engine.Level{}
	}
	if view.Book.Asks == nil {
		view.Book.Asks = []engine.Level{}
	}
	if view.Trades == nil {
		view.Trades = []engine.Trade{}
	}
	if view.Candles == nil {
		view.Candles = []Candle{}
	}
	if view.Accounts == nil {
		view.Accounts = []engine.Account{}
	}
	if view.Orders == nil {
		view.Orders = []engine.Order{}
	}
	if view.Account.OpenOrders == nil {
		view.Account.OpenOrders = []engine.Order{}
	}
	if view.Account.TradeHistory == nil {
		view.Account.TradeHistory = []engine.Trade{}
	}
	if view.Bots == nil {
		view.Bots = []Bot{}
	}
	if view.Events == nil {
		view.Events = []ScenarioMarker{}
	}
	return view
}

// CanonicalHash covers deterministic market and strategy state while excluding
// transport telemetry such as mode, running state, playback speed, and messages.
func (s *Simulation) CanonicalHash() (string, error) {
	type canonicalBot struct {
		ID       string
		Strategy string
		Bias     int64
		Params   StrategyParams
	}
	bots := make([]canonicalBot, len(s.Bots))
	for i, bot := range s.Bots {
		bots[i] = canonicalBot{ID: bot.ID, Strategy: bot.Strategy, Bias: bot.Bias, Params: bot.Params}
	}
	state := struct {
		Seed         int64
		Config       Config
		LogicalTime  int64
		Engine       engine.Snapshot
		Bots         []canonicalBot
		Candles      []Candle
		FairShift    int64
		StressUntil  int64
		DroughtUntil int64
	}{s.Seed, s.Config, s.LogicalTime, s.Engine.Snapshot(), bots, s.Candles, s.FairShift, s.StressUntil, s.DroughtUntil}
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:]), nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
