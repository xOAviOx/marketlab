package sim

import (
	"bytes"
	"encoding/json"
	"testing"

	"marketlab/internal/engine"
)

func TestSeededSimulationDeterministic(t *testing.T) {
	a, err := New("a", 424242)
	if err != nil { t.Fatal(err) }
	b, err := New("b", 424242)
	if err != nil { t.Fatal(err) }
	for range 80 { a.Step(); b.Step() }
	ha, _ := a.Engine.CanonicalHash()
	hb, _ := b.Engine.CanonicalHash()
	if ha != hb { t.Fatalf("same seed diverged: %s != %s", ha, hb) }
	if len(a.Events) == 0 || len(a.Engine.Snapshot().Trades) == 0 { t.Fatal("default seed did not produce active market") }
}

func TestScenariosUseOrdersAndReplayDeterministically(t *testing.T) {
	s, _ := New("live", 424242)
	for range 20 { s.Step() }
	before := len(s.Engine.Snapshot().Trades)
	if err := s.TriggerScenario("negative-news"); err != nil { t.Fatal(err) }
	for range 30 { s.Step() }
	if len(s.Engine.Snapshot().Trades) <= before { t.Fatal("negative news produced no actual trades with default seed") }
	want, _ := s.Engine.CanonicalHash()
	exp := s.Export()
	b, _ := json.Marshal(exp)
	replay, err := Import("replay", bytes.NewReader(b))
	if err != nil { t.Fatal(err) }
	if err := replay.SeekReplay(len(replay.ReplaySource)); err != nil { t.Fatal(err) }
	got, _ := replay.Engine.CanonicalHash()
	if want != got { t.Fatalf("replay hash mismatch\nwant %s\n got %s", want, got) }
}

func TestManualOrdersAndReplayReadOnly(t *testing.T) {
	s, _ := New("x", 7)
	r, err := s.SubmitManual(engine.SubmitRequest{Side: engine.Buy, Type: engine.Limit, Price: 9_000, Quantity: 5})
	if err != nil { t.Fatal(err) }
	if err := s.Cancel("MANUAL", HumanParticipant, r.Order.ID, "test"); err != nil { t.Fatal(err) }
	if err := s.EnterReplay(); err != nil { t.Fatal(err) }
	if _, err := s.SubmitManual(engine.SubmitRequest{Side: engine.Buy, Type: engine.Market, Quantity: 1}); err == nil { t.Fatal("replay accepted trading") }
}

func TestImportValidation(t *testing.T) {
	if _, err := Import("x", bytes.NewBufferString(`{"formatVersion":999}`)); err == nil { t.Fatal("accepted unsupported import") }
	tooLarge := bytes.Repeat([]byte("x"), MaxImportBytes+1)
	if _, err := Import("x", bytes.NewReader(tooLarge)); err == nil { t.Fatal("accepted oversized import") }
}
