package engine

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestNewRejectsInvalidEndowments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		endowments []Endowment
	}{
		{name: "empty"},
		{name: "empty participant", endowments: []Endowment{{Cash: 1}}},
		{name: "negative cash", endowments: []Endowment{{Participant: "a", Cash: -1}}},
		{name: "negative shares", endowments: []Endowment{{Participant: "a", Shares: -1}}},
		{name: "duplicate participant", endowments: []Endowment{{Participant: "a"}, {Participant: "a"}}},
		{name: "cash total overflow", endowments: []Endowment{{Participant: "a", Cash: math.MaxInt64}, {Participant: "b", Cash: 1}}},
		{name: "share total overflow", endowments: []Endowment{{Participant: "a", Shares: math.MaxInt64}, {Participant: "b", Shares: 1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.endowments); !errors.Is(err, ErrInvalidOrder) {
				t.Fatalf("New() error = %v, want ErrInvalidOrder", err)
			}
		})
	}
}

func TestRejectedSubmissionsAreAtomicAndConsumeCommandIDs(t *testing.T) {
	t.Parallel()
	e, err := New([]Endowment{{Participant: "buyer", Cash: 1_000}, {Participant: "seller", Shares: 10}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		request SubmitRequest
		wantErr error
	}{
		{request: SubmitRequest{Participant: "missing", Side: Buy, Type: Limit, Price: 1, Quantity: 1}, wantErr: ErrInvalidOrder},
		{request: SubmitRequest{Participant: "buyer", Side: "HOLD", Type: Limit, Price: 1, Quantity: 1}, wantErr: ErrInvalidOrder},
		{request: SubmitRequest{Participant: "buyer", Side: Buy, Type: "STOP", Price: 1, Quantity: 1}, wantErr: ErrInvalidOrder},
		{request: SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 1, Quantity: MaxQuantity + 1}, wantErr: ErrInvalidOrder},
		{request: SubmitRequest{Participant: "buyer", Side: Buy, Type: Market, Price: 1, Quantity: 1}, wantErr: ErrInvalidOrder},
		{request: SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 101, Quantity: 10}, wantErr: ErrInsufficientCash},
		{request: SubmitRequest{Participant: "seller", Side: Sell, Type: Market, Quantity: 11}, wantErr: ErrInsufficientShares},
	}
	for i, test := range tests {
		result, err := e.Submit(test.request)
		if !errors.Is(err, test.wantErr) {
			t.Fatalf("Submit(%+v) error = %v, want %v", test.request, err, test.wantErr)
		}
		if result.CommandID != uint64(i+1) {
			t.Fatalf("command ID = %d, want %d", result.CommandID, i+1)
		}
	}
	buyer, _ := e.Account("buyer")
	seller, _ := e.Account("seller")
	if buyer.CashAvailable != 1_000 || buyer.CashReserved != 0 || seller.SharesAvailable != 10 || seller.SharesReserved != 0 || len(e.Snapshot().Orders) != 0 {
		t.Fatalf("rejected commands mutated state: buyer=%+v seller=%+v orders=%+v", buyer, seller, e.Snapshot().Orders)
	}
	if err := e.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestPartialPriceImprovementLeavesOnlyRemainderReserved(t *testing.T) {
	t.Parallel()
	e, err := New([]Endowment{{Participant: "buyer", Cash: 1_000}, {Participant: "seller", Shares: 4}})
	if err != nil {
		t.Fatal(err)
	}
	ask := submit(t, e, SubmitRequest{Participant: "seller", Side: Sell, Type: Limit, Price: 80, Quantity: 4})
	bid := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 100, Quantity: 6})
	if bid.Order.Status != Partially || bid.Order.Filled != 4 || bid.Order.Remaining != 2 {
		t.Fatalf("bid = %+v, want partial 4 filled and 2 remaining", bid.Order)
	}
	if len(bid.Trades) != 1 || bid.Trades[0].Price != 80 || bid.Trades[0].SellOrderID != ask.Order.ID {
		t.Fatalf("trades = %+v", bid.Trades)
	}
	buyer, _ := e.Account("buyer")
	if buyer.CashAvailable != 480 || buyer.CashReserved != 200 || buyer.SharesAvailable != 4 {
		t.Fatalf("buyer settlement = %+v, want available=480 reserved=200 shares=4", buyer)
	}
	if _, err := e.Cancel("buyer", bid.Order.ID); err != nil {
		t.Fatal(err)
	}
	buyer, _ = e.Account("buyer")
	if buyer.CashAvailable != 680 || buyer.CashReserved != 0 {
		t.Fatalf("buyer after cancel = %+v, want available=680 reserved=0", buyer)
	}
	if got := e.Snapshot().Sequence; got != 4 {
		t.Fatalf("sequence = %d, want 4", got)
	}
}

func TestMarketBuyStopsAtWholeShareAffordability(t *testing.T) {
	t.Parallel()
	e, err := New([]Endowment{{Participant: "buyer", Cash: 250}, {Participant: "seller", Shares: 3}})
	if err != nil {
		t.Fatal(err)
	}
	ask := submit(t, e, SubmitRequest{Participant: "seller", Side: Sell, Type: Limit, Price: 100, Quantity: 3})
	result := submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Market, Quantity: 4})
	if result.Order.Status != Canceled || result.Order.Filled != 2 || result.Unfilled != 2 || result.Reason != ReasonInsufficientCash {
		t.Fatalf("market result = %+v", result)
	}
	if len(result.Trades) != 1 || result.Trades[0].Quantity != 2 {
		t.Fatalf("trades = %+v", result.Trades)
	}
	buyer, _ := e.Account("buyer")
	if buyer.CashAvailable != 50 || buyer.SharesAvailable != 2 {
		t.Fatalf("buyer = %+v", buyer)
	}
	remaining, _ := e.Order(ask.Order.ID)
	if remaining.Status != Partially || remaining.Remaining != 1 {
		t.Fatalf("resting ask = %+v", remaining)
	}
}

func TestSelfTradeAfterExternalFillReleasesAggressorRemainder(t *testing.T) {
	t.Parallel()
	e, err := New([]Endowment{{Participant: "alice", Cash: 1_000, Shares: 5}, {Participant: "bob", Shares: 1}})
	if err != nil {
		t.Fatal(err)
	}
	bobAsk := submit(t, e, SubmitRequest{Participant: "bob", Side: Sell, Type: Limit, Price: 90, Quantity: 1})
	aliceAsk := submit(t, e, SubmitRequest{Participant: "alice", Side: Sell, Type: Limit, Price: 100, Quantity: 2})
	result := submit(t, e, SubmitRequest{Participant: "alice", Side: Buy, Type: Limit, Price: 100, Quantity: 4})
	if result.Order.Status != Canceled || result.Order.Filled != 1 || result.Order.Remaining != 3 || result.Reason != ReasonSelfTrade {
		t.Fatalf("aggressor = %+v, reason=%q", result.Order, result.Reason)
	}
	if len(result.Trades) != 1 || result.Trades[0].SellOrderID != bobAsk.Order.ID {
		t.Fatalf("trades = %+v", result.Trades)
	}
	if own, _ := e.Order(aliceAsk.Order.ID); own.Status != Open || own.Remaining != 2 {
		t.Fatalf("own resting order changed: %+v", own)
	}
	alice, _ := e.Account("alice")
	if alice.CashAvailable != 910 || alice.CashReserved != 0 || alice.SharesAvailable != 4 || alice.SharesReserved != 2 {
		t.Fatalf("Alice account = %+v", alice)
	}
}

func TestCanonicalStateIncludesCountersAndReturnsCopies(t *testing.T) {
	t.Parallel()
	left, err := New([]Endowment{{Participant: "z", Shares: 2}, {Participant: "a", Cash: 1_000}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := New([]Endowment{{Participant: "a", Cash: 1_000}, {Participant: "z", Shares: 2}})
	if err != nil {
		t.Fatal(err)
	}
	requests := []SubmitRequest{
		{Participant: "z", Side: Sell, Type: Limit, Price: 100, Quantity: 2},
		{Participant: "a", Side: Buy, Type: Limit, Price: 110, Quantity: 1},
	}
	for _, request := range requests {
		submit(t, left, request)
		submit(t, right, request)
	}
	snapshot := left.Snapshot()
	if !reflect.DeepEqual(snapshot, right.Snapshot()) {
		t.Fatal("equivalent replay snapshots differ")
	}
	if snapshot.Accounts == nil || snapshot.Orders == nil || snapshot.Trades == nil || snapshot.Bids == nil || snapshot.Asks == nil {
		t.Fatal("canonical snapshot contains a nil slice")
	}
	leftHash, err := left.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	rightHash, _ := right.CanonicalHash()
	if leftHash != rightHash || len(leftHash) != 64 {
		t.Fatalf("hashes = %q and %q", leftHash, rightHash)
	}

	before := left.Snapshot()
	snapshot.Accounts[0].CashAvailable++
	snapshot.Orders[0].Remaining++
	snapshot.Trades[0].Quantity++
	if !reflect.DeepEqual(left.Snapshot(), before) {
		t.Fatal("mutating a snapshot changed engine state")
	}

	empty1, _ := New([]Endowment{{Participant: "a", Cash: 1}})
	empty2, _ := New([]Endowment{{Participant: "a", Cash: 1}})
	if _, err := empty1.Submit(SubmitRequest{Participant: "a", Side: Buy}); err == nil {
		t.Fatal("invalid order succeeded")
	}
	hash1, _ := empty1.CanonicalHash()
	hash2, _ := empty2.CanonicalHash()
	if hash1 == hash2 {
		t.Fatal("canonical hash omitted rejected-command history")
	}
}

func TestCounterCapacityGuardsAreAtomic(t *testing.T) {
	t.Parallel()
	request := SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 1, Quantity: 1}
	for _, test := range []struct {
		name    string
		corrupt func(*Engine)
	}{
		{name: "command", corrupt: func(e *Engine) { e.nextCommandID = math.MaxUint64 }},
		{name: "order", corrupt: func(e *Engine) { e.nextOrderID = math.MaxUint64 }},
		{name: "sequence", corrupt: func(e *Engine) { e.sequence = math.MaxUint64 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, err := New([]Endowment{{Participant: "buyer", Cash: 10}})
			if err != nil {
				t.Fatal(err)
			}
			test.corrupt(e)
			if _, err := e.Submit(request); !errors.Is(err, ErrCapacity) {
				t.Fatalf("Submit() error = %v, want ErrCapacity", err)
			}
			account, _ := e.Account("buyer")
			if account.CashAvailable != 10 || account.CashReserved != 0 || len(e.orders) != 0 {
				t.Fatalf("capacity failure mutated order state: account=%+v orders=%+v", account, e.orders)
			}
		})
	}
}

func TestInvariantCheckerDetectsBookAndReservationCorruption(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		corrupt func(*Engine)
		want    string
	}{
		{name: "reservation", corrupt: func(e *Engine) {
			e.accounts["buyer"].CashReserved--
			e.accounts["buyer"].CashAvailable++
		}, want: "reservation mismatch"},
		{name: "duplicate queue entry", corrupt: func(e *Engine) { e.bids[10] = append(e.bids[10], 1) }, want: "non-FIFO"},
		{name: "wrong level", corrupt: func(e *Engine) { e.bids[11] = e.bids[10]; delete(e.bids, 10) }, want: "invalid order"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, err := New([]Endowment{{Participant: "buyer", Cash: 100}})
			if err != nil {
				t.Fatal(err)
			}
			submit(t, e, SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 10, Quantity: 2})
			test.corrupt(e)
			if err := e.CheckInvariants(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CheckInvariants() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func BenchmarkRestingLimitAndCancel(b *testing.B) {
	e, err := New([]Endowment{{Participant: "buyer", Cash: math.MaxInt64}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := e.Submit(SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: 100, Quantity: 1})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := e.Cancel("buyer", result.Order.ID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSnapshotThousandOrders(b *testing.B) {
	e, err := New([]Endowment{{Participant: "buyer", Cash: 1_000_000_000}})
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1_000; i++ {
		if _, err := e.Submit(SubmitRequest{Participant: "buyer", Side: Buy, Type: Limit, Price: int64(i + 1), Quantity: 1}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.Snapshot()
	}
}
