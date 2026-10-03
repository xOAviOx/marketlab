package engine

import (
	"errors"
	"math"
	"math/rand"
	"testing"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New([]Endowment{
		{Participant: "buyer", Cash: 10_000_000, Shares: 1_000},
		{Participant: "seller1", Cash: 10_000_000, Shares: 1_000},
		{Participant: "seller2", Cash: 10_000_000, Shares: 1_000},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func submit(t *testing.T, e *Engine, req SubmitRequest) Result {
	t.Helper()
	r, err := e.Submit(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPricePriorityAndRestingPrice(t *testing.T) {
	e := testEngine(t)
	low := submit(t, e, SubmitRequest{Participant: "seller1", Side: Sell, Type: Limit, Price: 10_100, Quantity: 4})
	high := submit(t, e, SubmitRequest{Participant: "seller2", Side: Sell, Type: Limit, Price: 10_000, Quantity: 4})
	r := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 10_200, Quantity: 6})
	if len(r.Trades) != 2 || r.Trades[0].SellOrderID != high.Order.ID || r.Trades[0].Price != 10_000 || r.Trades[1].SellOrderID != low.Order.ID || r.Trades[1].Price != 10_100 {
		t.Fatalf("unexpected price priority/trades: %+v", r.Trades)
	}
}

func TestFIFOAtPrice(t *testing.T) {
	e := testEngine(t)
	first := submit(t, e, SubmitRequest{Participant: "seller1", Side: Sell, Type: Limit, Price: 10_000, Quantity: 2})
	second := submit(t, e, SubmitRequest{Participant: "seller2", Side: Sell, Type: Limit, Price: 10_000, Quantity: 2})
	r := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Market, Quantity: 3})
	if len(r.Trades) != 2 || r.Trades[0].SellOrderID != first.Order.ID || r.Trades[1].SellOrderID != second.Order.ID {
		t.Fatalf("FIFO violated: %+v", r.Trades)
	}
}

func TestPartialAcrossLevelsAndCancel(t *testing.T) {
	e := testEngine(t)
	first := submit(t, e, SubmitRequest{Participant: "seller1", Side: Sell, Type: Limit, Price: 9_900, Quantity: 2})
	submit(t, e, SubmitRequest{Participant: "seller2", Side: Sell, Type: Limit, Price: 10_000, Quantity: 3})
	r := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 10_000, Quantity: 4})
	if r.Order.Status != Filled || len(r.Trades) != 2 || r.Trades[0].Quantity != 2 || r.Trades[1].Quantity != 2 {
		t.Fatalf("unexpected partial fills: %+v", r)
	}
	if _, err := e.Cancel("seller1", first.Order.ID); !errors.Is(err, ErrOrderNotOpen) {
		t.Fatalf("filled order cancel error = %v", err)
	}
	remaining := e.Snapshot().Orders[1]
	if remaining.Remaining != 1 || remaining.Status != Partially {
		t.Fatalf("maker remainder wrong: %+v", remaining)
	}
	before, _ := e.Account("seller2")
	if _, err := e.Cancel("seller2", remaining.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Account("seller2")
	if after.SharesReserved != 0 || after.SharesAvailable != before.SharesAvailable+1 {
		t.Fatalf("reservation not released: before=%+v after=%+v", before, after)
	}
}

func TestReservationAndPriceImprovementRefund(t *testing.T) {
	e := testEngine(t)
	submit(t, e, SubmitRequest{Participant: "seller1", Side: Sell, Type: Limit, Price: 9_500, Quantity: 5})
	before, _ := e.Account("buyer")
	r := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 10_000, Quantity: 5})
	after, _ := e.Account("buyer")
	if r.Trades[0].Price != 9_500 || after.CashAvailable != before.CashAvailable-47_500 || after.CashReserved != 0 || after.SharesAvailable != before.SharesAvailable+5 {
		t.Fatalf("price improvement settlement incorrect: before=%+v after=%+v", before, after)
	}

	resting := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 9_000, Quantity: 2})
	a, _ := e.Account("buyer")
	if a.CashReserved != 18_000 {
		t.Fatalf("cash reserve = %d", a.CashReserved)
	}
	if _, err := e.Cancel("buyer", resting.Order.ID); err != nil {
		t.Fatal(err)
	}
	a, _ = e.Account("buyer")
	if a.CashReserved != 0 {
		t.Fatalf("cash reserve after cancel = %d", a.CashReserved)
	}
}
func TestInsufficientBalancesAndMarketRemainders(t *testing.T) {
	e := testEngine(t)
	if _, err := e.Submit(SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: MaxPrice, Quantity: MaxQuantity}); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("expected cash rejection, got %v", err)
	}
	if _, err := e.Submit(SubmitRequest{Participant: "seller1", Side: Sell, Type: Market, Quantity: 1_001}); !errors.Is(err, ErrInsufficientShares) {
		t.Fatalf("expected shares rejection, got %v", err)
	}
	empty := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Market, Quantity: 10})
	if empty.Order.Status != Canceled || empty.Unfilled != 10 || len(empty.Trades) != 0 {
		t.Fatalf("empty market result: %+v", empty)
	}
	submit(t, e, SubmitRequest{Participant: "seller1", Side: Sell, Type: Limit, Price: 10_000, Quantity: 2})
	partial := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Market, Quantity: 5})
	if partial.Order.Status != Canceled || partial.Order.Filled != 2 || partial.Unfilled != 3 {
		t.Fatalf("partial market result: %+v", partial)
	}
}
func TestSelfTradePreventionCancelsIncoming(t *testing.T) {
	e := testEngine(t)
	submit(t, e, SubmitRequest{Participant: "buyer", Side: Sell, Type: Limit, Price: 10_000, Quantity: 2})
	r := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 10_000, Quantity: 2})
	if r.Order.Status != Canceled || r.Unfilled != 2 || len(r.Trades) != 0 || r.Reason == "" {
		t.Fatalf("self trade policy failed: %+v", r)
	}
	a, _ := e.Account("buyer")
	if a.CashReserved != 0 || a.SharesReserved != 2 {
		t.Fatalf("incoming reservation leak: %+v", a)
	}
}

func TestInvalidAndOverflowInputs(t *testing.T) {
	e := testEngine(t)
	cases := []SubmitRequest{
		{Participant: "buyer", Side: Buy, Type: Limit, Price: 0, Quantity: 1},
		{Participant: "buyer", Side: Buy, Type: Limit, Price: MaxPrice + 1, Quantity: 1},
		{Participant: "buyer", Side: Buy, Type: Limit, Price: 100, Quantity: 0},
		{Participant: "buyer", Side: Buy, Type: Limit, Price: math.MaxInt64, Quantity: 2},
		{Participant: "buyer", Side: Side("NOPE"), Type: Limit, Price: 100, Quantity: 1},
		{Participant: "buyer", Side: Buy, Type: Market, Price: 100, Quantity: 1},
	}
	for _, tc := range cases {
		if _, err := e.Submit(tc); !errors.Is(err, ErrInvalidOrder) {
			t.Errorf("request %+v error = %v", tc, err)
		}
	}
}

func TestDeterministicReplayAndRandomInvariants(t *testing.T) {
	commands := []SubmitRequest{
		{Participant: "seller1", Side: Sell, Type: Limit, Price: 10_100, Quantity: 10},
		{Participant: "buyer", Side: Buy, Type: Limit, Price: 9_900, Quantity: 20},
		{Participant: "seller2", Side: Sell, Type: Market, Quantity: 5},
		{Participant: "buyer", Side: Buy, Type: Market, Quantity: 7},
	}
	hashes := make([]string, 2)
	for i := range 2 {
		e := testEngine(t)
		for _, cmd := range commands {
			submit(t, e, cmd)
		}
		hashes[i], _ = e.CanonicalHash()
	}
	if hashes[0] != hashes[1] {
		t.Fatalf("deterministic hashes differ: %s %s", hashes[0], hashes[1])
	}

	e := testEngine(t)
	rng := rand.New(rand.NewSource(42))
	participants := []string{"buyer", "seller1", "seller2"}
	for range 1_000 {
		p := participants[rng.Intn(len(participants))]
		side := Buy
		if rng.Intn(2) == 0 {
			side = Sell
		}
		typ := Limit
		price := int64(9_500 + rng.Intn(1_001))
		if rng.Intn(5) == 0 {
			typ, price = Market, 0
		}
		_, _ = e.Submit(SubmitRequest{Participant: p, Side: side, Type: typ, Price: price, Quantity: int64(1 + rng.Intn(5))})
		if err := e.CheckInvariants(); err != nil {
			t.Fatalf("invariant failed: %v", err)
		}
	}
}

func FuzzEngineInvariants(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(99))
	f.Fuzz(func(t *testing.T, seed int64) {
		e := testEngine(t)
		rng := rand.New(rand.NewSource(seed))
		for range 100 {
			p := []string{"buyer", "seller1", "seller2"}[rng.Intn(3)]
			side := []Side{Buy, Sell}[rng.Intn(2)]
			_, _ = e.Submit(SubmitRequest{Participant: p, Side: side, Type: Limit, Price: int64(9_000 + rng.Intn(2_001)), Quantity: int64(1 + rng.Intn(10))})
			if err := e.CheckInvariants(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func BenchmarkMatching(b *testing.B) {
	for i := 0; i < b.N; i++ {
		e, _ := New([]Endowment{{Participant: "buyer", Cash: 1_000_000_000, Shares: 0}, {Participant: "seller", Cash: 0, Shares: 100_000}})
		for j := 0; j < 100; j++ {
			_, _ = e.Submit(SubmitRequest{Participant: "seller", Side: Sell, Type: Limit, Price: 10_000 + int64(j), Quantity: 10})
		}
		for j := 0; j < 100; j++ {
			_, _ = e.Submit(SubmitRequest{Participant: "buyer", Side: Buy, Type: Market, Quantity: 10})
		}
	}
}
