package sim

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"marketlab/internal/engine"
)

func TestSeededSimulationDeterministic(t *testing.T) {
	a, err := New("a", 424242)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("b", 424242)
	if err != nil {
		t.Fatal(err)
	}
	for range 80 {
		a.Step()
		b.Step()
	}
	ha, _ := a.Engine.CanonicalHash()
	hb, _ := b.Engine.CanonicalHash()
	if ha != hb {
		t.Fatalf("same seed diverged: %s != %s", ha, hb)
	}
	if len(a.Events) == 0 || len(a.Engine.Snapshot().Trades) == 0 {
		t.Fatal("default seed did not produce active market")
	}
}

func TestSnapshotJSONUsesFrontendBookAndArrayContract(t *testing.T) {
	s, err := New("snapshot", 7)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	book := payload["book"].(map[string]any)
	if _, ok := book["bids"].([]any); !ok {
		t.Fatalf("book bids must be an array: %s", body)
	}
	if _, ok := book["asks"].([]any); !ok {
		t.Fatalf("book asks must be an array: %s", body)
	}
	if _, ok := payload["events"].([]any); !ok {
		t.Fatalf("events must be an array: %s", body)
	}
	account := payload["account"].(map[string]any)
	if _, ok := account["openOrders"].([]any); !ok {
		t.Fatalf("openOrders must be an array: %s", body)
	}
	if _, ok := account["tradeHistory"].([]any); !ok {
		t.Fatalf("tradeHistory must be an array: %s", body)
	}
}

func TestScenariosUseOrdersAndReplayDeterministically(t *testing.T) {
	s, _ := New("live", 424242)
	for range 20 {
		s.Step()
	}
	before := len(s.Engine.Snapshot().Trades)
	if err := s.TriggerScenario("negative-news"); err != nil {
		t.Fatal(err)
	}
	for range 30 {
		s.Step()
	}
	if len(s.Engine.Snapshot().Trades) <= before {
		t.Fatal("negative news produced no actual trades with default seed")
	}
	want, _ := s.Engine.CanonicalHash()
	exp := s.Export()
	b, _ := json.Marshal(exp)
	replay, err := Import("replay", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekReplay(len(replay.ReplaySource)); err != nil {
		t.Fatal(err)
	}
	got, _ := replay.Engine.CanonicalHash()
	if want != got {
		t.Fatalf("replay hash mismatch\nwant %s\n got %s", want, got)
	}
}

func TestManualOrdersAndReplayReadOnly(t *testing.T) {
	s, _ := New("x", 7)
	r, err := s.SubmitManual(engine.SubmitRequest{Side: engine.Buy, Type: engine.Limit, Price: 9_000, Quantity: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel("MANUAL", HumanParticipant, r.Order.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnterReplay(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitManual(engine.SubmitRequest{Side: engine.Buy, Type: engine.Market, Quantity: 1}); err == nil {
		t.Fatal("replay accepted trading")
	}
}

func TestImportValidation(t *testing.T) {
	if _, err := Import("x", bytes.NewBufferString(`{"formatVersion":999}`)); err == nil {
		t.Fatal("accepted unsupported import")
	}
	tooLarge := bytes.Repeat([]byte("x"), MaxImportBytes+1)
	if _, err := Import("x", bytes.NewReader(tooLarge)); err == nil {
		t.Fatal("accepted oversized import")
	}
}

func TestDirectionalStrategiesEnforceInventoryRiskLimit(t *testing.T) {
	t.Run("mean reversion", func(t *testing.T) {
		s, err := newSimulation("risk-mean", 11, DefaultConfig(), defaultEndowments())
		if err != nil {
			t.Fatal(err)
		}
		bot := findBot(t, s, "reversion-one")
		bot.Params.Size, bot.Params.RiskLimit, bot.Params.Threshold = 12, 5, 8
		if _, err := s.submit("TEST", "low offer", engine.SubmitRequest{Participant: "scenario-fund", Side: engine.Sell, Type: engine.Limit, Price: 9_000, Quantity: 100}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.submit("TEST", "establish low last", engine.SubmitRequest{Participant: HumanParticipant, Side: engine.Buy, Type: engine.Market, Quantity: 1}); err != nil {
			t.Fatal(err)
		}
		s.runMeanReversion(bot)
		assertRiskBoundedOrder(t, s, bot, 2_005, 5)
	})

	t.Run("momentum", func(t *testing.T) {
		s, err := newSimulation("risk-momentum", 12, DefaultConfig(), defaultEndowments())
		if err != nil {
			t.Fatal(err)
		}
		bot := findBot(t, s, "momentum-one")
		bot.Params.Size, bot.Params.RiskLimit, bot.Params.Lookback, bot.Params.Threshold = 12, 3, 2, 4
		for i, price := range []int64{9_000, 9_100} {
			quantity := int64(100)
			if i == 0 {
				quantity = 1
			}
			if _, err := s.submit("TEST", "trend offer", engine.SubmitRequest{Participant: "scenario-fund", Side: engine.Sell, Type: engine.Limit, Price: price, Quantity: quantity}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.submit("TEST", "establish trend", engine.SubmitRequest{Participant: HumanParticipant, Side: engine.Buy, Type: engine.Market, Quantity: 1}); err != nil {
				t.Fatal(err)
			}
		}
		s.runMomentum(bot)
		assertRiskBoundedOrder(t, s, bot, 2_003, 3)
	})
}

func findBot(t *testing.T, s *Simulation, id string) *Bot {
	t.Helper()
	for i := range s.Bots {
		if s.Bots[i].ID == id {
			return &s.Bots[i]
		}
	}
	t.Fatalf("bot %q not found", id)
	return nil
}

func assertRiskBoundedOrder(t *testing.T, s *Simulation, bot *Bot, wantInventory, wantQuantity int64) {
	t.Helper()
	account, _ := s.Engine.Account(bot.ID)
	inventory := account.SharesAvailable + account.SharesReserved
	if inventory != wantInventory {
		t.Fatalf("inventory = %d, want %d", inventory, wantInventory)
	}
	event := s.Events[len(s.Events)-1]
	if event.Request == nil || event.Request.Quantity != wantQuantity {
		t.Fatalf("risk-bounded event request = %+v, want quantity %d", event.Request, wantQuantity)
	}
	if event.ReasonFacts["inventory"] != 2_000 || event.ReasonFacts["riskUpper"] != wantInventory || event.ReasonFacts["quantity"] != wantQuantity {
		t.Fatalf("risk reason facts are not factual: %+v", event.ReasonFacts)
	}
}

func TestStrategyChangesAreStructuredAndReplayed(t *testing.T) {
	s, err := New("strategy-live", 31)
	if err != nil {
		t.Fatal(err)
	}
	params := &StrategyParams{Size: 7, Interval: 2_750, Lookback: 9, Threshold: 22, RiskLimit: 77}
	if err := s.SetStrategy("Momentum", false, params); err != nil {
		t.Fatal(err)
	}
	event := s.Events[len(s.Events)-1]
	if event.Kind != "STRATEGY" || len(event.StrategyChanges) != 2 {
		t.Fatalf("strategy event is not structured: %+v", event)
	}

	liveHash, err := s.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := Import("strategy-replay", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekReplay(len(replay.ReplaySource)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"momentum-one", "momentum-two"} {
		live, replayed := findBot(t, s, id), findBot(t, replay, id)
		if !reflect.DeepEqual(live.Params, replayed.Params) {
			t.Fatalf("%s params diverged: live=%+v replay=%+v", id, live.Params, replayed.Params)
		}
	}
	replayHash, err := replay.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if liveHash != replayHash {
		t.Fatalf("canonical state diverged after strategy replay: %s != %s", liveHash, replayHash)
	}
}
