// Package engine implements a deterministic, single-owner limit order book.
// All monetary values are integer cents and all quantities are integer shares.
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	Symbol      = "NOVA"
	MaxPrice    = int64(1_000_000_000)
	MaxQuantity = int64(1_000_000)
)

type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

type OrderType string

const (
	Limit  OrderType = "LIMIT"
	Market OrderType = "MARKET"
)

type OrderStatus string

const (
	Open      OrderStatus = "OPEN"
	Partially OrderStatus = "PARTIALLY_FILLED"
	Filled    OrderStatus = "FILLED"
	Canceled  OrderStatus = "CANCELED"
)

const (
	ReasonSelfTrade        = "self-trade prevented; aggressing remainder canceled"
	ReasonMarketUnfilled   = "unfilled market remainder canceled"
	ReasonInsufficientCash = "unfilled remainder exceeds available cash"
)

var (
	ErrInvalidOrder       = errors.New("invalid order")
	ErrInsufficientCash   = errors.New("insufficient available cash")
	ErrInsufficientShares = errors.New("insufficient available shares")
	ErrOrderNotFound      = errors.New("order not found")
	ErrNotOrderOwner      = errors.New("order belongs to another participant")
	ErrOrderNotOpen       = errors.New("order is not open")
	ErrCapacity           = errors.New("engine counter capacity exhausted")
)

type Account struct {
	Participant     string `json:"participant"`
	CashAvailable   int64  `json:"cashAvailable"`
	CashReserved    int64  `json:"cashReserved"`
	SharesAvailable int64  `json:"sharesAvailable"`
	SharesReserved  int64  `json:"sharesReserved"`
}

type Endowment struct {
	Participant string `json:"participant"`
	Cash        int64  `json:"cash"`
	Shares      int64  `json:"shares"`
}

type Order struct {
	ID          uint64      `json:"id"`
	CommandID   uint64      `json:"commandId"`
	Participant string      `json:"participant"`
	Side        Side        `json:"side"`
	Type        OrderType   `json:"type"`
	Price       int64       `json:"price"`
	Quantity    int64       `json:"quantity"`
	Filled      int64       `json:"filled"`
	Remaining   int64       `json:"remaining"`
	Status      OrderStatus `json:"status"`
	Sequence    uint64      `json:"sequence"`
}

type Trade struct {
	ID            uint64 `json:"id"`
	BuyOrderID    uint64 `json:"buyOrderId"`
	SellOrderID   uint64 `json:"sellOrderId"`
	Buyer         string `json:"buyer"`
	Seller        string `json:"seller"`
	Price         int64  `json:"price"`
	Quantity      int64  `json:"quantity"`
	AggressorSide Side   `json:"aggressorSide"`
	Sequence      uint64 `json:"sequence"`
}

type SubmitRequest struct {
	Participant string    `json:"participant"`
	Side        Side      `json:"side"`
	Type        OrderType `json:"type"`
	Price       int64     `json:"price,omitempty"`
	Quantity    int64     `json:"quantity"`
}

type Result struct {
	CommandID uint64  `json:"commandId"`
	Order     Order   `json:"order"`
	Trades    []Trade `json:"trades"`
	Unfilled  int64   `json:"unfilled"`
	Reason    string  `json:"reason,omitempty"`
}

type Level struct {
	Price    int64 `json:"price"`
	Quantity int64 `json:"quantity"`
	Orders   int   `json:"orders"`
}

// Snapshot is a canonical representation of the complete externally relevant
// engine state. Its slices are always non-nil and have deterministic ordering.
type Snapshot struct {
	Sequence      uint64    `json:"sequence"`
	NextCommandID uint64    `json:"nextCommandId"`
	NextOrderID   uint64    `json:"nextOrderId"`
	NextTradeID   uint64    `json:"nextTradeId"`
	TotalCash     int64     `json:"totalCash"`
	TotalShares   int64     `json:"totalShares"`
	Accounts      []Account `json:"accounts"`
	Orders        []Order   `json:"orders"`
	Trades        []Trade   `json:"trades"`
	Bids          []Level   `json:"bids"`
	Asks          []Level   `json:"asks"`
}

// Engine is not internally synchronized. A simulation should give one owner
// exclusive access and serialize Submit, Cancel, and snapshot operations.
type Engine struct {
	accounts      map[string]*Account
	orders        map[uint64]*Order
	bids          map[int64][]uint64
	asks          map[int64][]uint64
	trades        []Trade
	sequence      uint64
	nextCommandID uint64
	nextOrderID   uint64
	nextTradeID   uint64
	totalCash     int64
	totalShares   int64
}

func New(endowments []Endowment) (*Engine, error) {
	e := &Engine{
		accounts:      make(map[string]*Account, len(endowments)),
		orders:        make(map[uint64]*Order),
		bids:          make(map[int64][]uint64),
		asks:          make(map[int64][]uint64),
		trades:        make([]Trade, 0),
		nextCommandID: 1,
		nextOrderID:   1,
		nextTradeID:   1,
	}
	if len(endowments) == 0 {
		return nil, fmt.Errorf("%w: at least one participant is required", ErrInvalidOrder)
	}
	for _, x := range endowments {
		if x.Participant == "" || x.Cash < 0 || x.Shares < 0 {
			return nil, fmt.Errorf("%w: invalid endowment for %q", ErrInvalidOrder, x.Participant)
		}
		if _, exists := e.accounts[x.Participant]; exists {
			return nil, fmt.Errorf("%w: duplicate participant %q", ErrInvalidOrder, x.Participant)
		}
		cash, ok := addInt64(e.totalCash, x.Cash)
		if !ok {
			return nil, fmt.Errorf("%w: cash endowment total overflow", ErrInvalidOrder)
		}
		shares, ok := addInt64(e.totalShares, x.Shares)
		if !ok {
			return nil, fmt.Errorf("%w: share endowment total overflow", ErrInvalidOrder)
		}
		e.accounts[x.Participant] = &Account{
			Participant:     x.Participant,
			CashAvailable:   x.Cash,
			SharesAvailable: x.Shares,
		}
		e.totalCash = cash
		e.totalShares = shares
	}
	return e, nil
}

func addInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func checkedValue(price, quantity int64) (int64, error) {
	if price <= 0 || price > MaxPrice {
		return 0, fmt.Errorf("%w: price must be between 1 and %d cents", ErrInvalidOrder, MaxPrice)
	}
	if quantity <= 0 || quantity > MaxQuantity {
		return 0, fmt.Errorf("%w: quantity must be between 1 and %d", ErrInvalidOrder, MaxQuantity)
	}
	if price > math.MaxInt64/quantity {
		return 0, fmt.Errorf("%w: order value overflow", ErrInvalidOrder)
	}
	return price * quantity, nil
}

func (e *Engine) takeCommandID() (uint64, error) {
	if e == nil || e.nextCommandID == 0 || e.nextCommandID == math.MaxUint64 {
		return 0, ErrCapacity
	}
	id := e.nextCommandID
	e.nextCommandID++
	return id, nil
}

// Submit validates and synchronously processes an order. Limit buys reserve
// limit price times quantity and refund price improvement as each fill settles.
// Limit sells reserve shares. Market orders never rest; a market buy spends only
// through the last whole share it can afford at the current resting price.
func (e *Engine) Submit(req SubmitRequest) (Result, error) {
	commandID, err := e.takeCommandID()
	if err != nil {
		return Result{}, err
	}
	result := Result{CommandID: commandID, Trades: make([]Trade, 0)}

	account := e.accounts[req.Participant]
	if account == nil {
		return result, fmt.Errorf("%w: unknown participant %q", ErrInvalidOrder, req.Participant)
	}
	if req.Side != Buy && req.Side != Sell {
		return result, fmt.Errorf("%w: side must be BUY or SELL", ErrInvalidOrder)
	}
	if req.Type != Limit && req.Type != Market {
		return result, fmt.Errorf("%w: type must be LIMIT or MARKET", ErrInvalidOrder)
	}
	if req.Quantity <= 0 || req.Quantity > MaxQuantity {
		return result, fmt.Errorf("%w: quantity must be between 1 and %d", ErrInvalidOrder, MaxQuantity)
	}
	var orderValue int64
	if req.Type == Limit {
		orderValue, err = checkedValue(req.Price, req.Quantity)
		if err != nil {
			return result, err
		}
	} else if req.Price != 0 {
		return result, fmt.Errorf("%w: market orders must have price zero", ErrInvalidOrder)
	}

	// One sequence is used by the order and at most one by every existing order
	// it can fill. Reserve capacity before mutating balances so overflow cannot
	// leave a partially applied command.
	maximumEvents := uint64(len(e.orders)) + 1
	if e.nextOrderID == 0 || e.nextOrderID == math.MaxUint64 ||
		e.sequence > math.MaxUint64-maximumEvents ||
		e.nextTradeID == 0 || e.nextTradeID > math.MaxUint64-uint64(len(e.orders)) {
		return result, ErrCapacity
	}
	if req.Side == Buy && req.Type == Limit {
		if account.CashAvailable < orderValue {
			return result, fmt.Errorf("%w: need %d cents, have %d", ErrInsufficientCash, orderValue, account.CashAvailable)
		}
		account.CashAvailable -= orderValue
		account.CashReserved += orderValue
	}
	if req.Side == Sell {
		if account.SharesAvailable < req.Quantity {
			return result, fmt.Errorf("%w: need %d shares, have %d", ErrInsufficientShares, req.Quantity, account.SharesAvailable)
		}
		if req.Type == Limit {
			account.SharesAvailable -= req.Quantity
			account.SharesReserved += req.Quantity
		}
	}

	e.sequence++
	order := &Order{
		ID:          e.nextOrderID,
		CommandID:   commandID,
		Participant: req.Participant,
		Side:        req.Side,
		Type:        req.Type,
		Price:       req.Price,
		Quantity:    req.Quantity,
		Remaining:   req.Quantity,
		Status:      Open,
		Sequence:    e.sequence,
	}
	e.nextOrderID++
	e.orders[order.ID] = order

	for order.Remaining > 0 {
		maker := e.bestMatch(order)
		if maker == nil {
			break
		}
		if maker.Participant == order.Participant {
			result.Reason = ReasonSelfTrade
			break
		}
		quantity := min(order.Remaining, maker.Remaining)
		if order.Side == Buy && order.Type == Market {
			quantity = min(quantity, account.CashAvailable/maker.Price)
			if quantity == 0 {
				result.Reason = ReasonInsufficientCash
				break
			}
		}
		result.Trades = append(result.Trades, e.execute(order, maker, quantity))
	}

	switch {
	case order.Remaining == 0:
		order.Status = Filled
	case order.Type == Limit && result.Reason == "":
		if order.Filled > 0 {
			order.Status = Partially
		}
		e.rest(order)
	default:
		e.release(order)
		order.Status = Canceled
		if result.Reason == "" {
			result.Reason = ReasonMarketUnfilled
		}
	}
	result.Order = *order
	result.Unfilled = order.Remaining
	return result, nil
}

func (e *Engine) bestMatch(incoming *Order) *Order {
	book := e.asks
	if incoming.Side == Sell {
		book = e.bids
	}
	var bestPrice int64
	found := false
	for price, queue := range book {
		if len(queue) == 0 {
			continue
		}
		if incoming.Type == Limit && (incoming.Side == Buy && price > incoming.Price || incoming.Side == Sell && price < incoming.Price) {
			continue
		}
		if !found || incoming.Side == Buy && price < bestPrice || incoming.Side == Sell && price > bestPrice {
			bestPrice = price
			found = true
		}
	}
	if !found {
		return nil
	}
	queue := book[bestPrice]
	for len(queue) > 0 {
		order := e.orders[queue[0]]
		if order != nil && (order.Status == Open || order.Status == Partially) && order.Remaining > 0 {
			book[bestPrice] = queue
			return order
		}
		queue = queue[1:]
	}
	delete(book, bestPrice)
	return e.bestMatch(incoming)
}

func (e *Engine) execute(incoming, maker *Order, quantity int64) Trade {
	price := maker.Price
	buyer, seller := incoming, maker
	if incoming.Side == Sell {
		buyer, seller = maker, incoming
	}
	buyerAccount := e.accounts[buyer.Participant]
	sellerAccount := e.accounts[seller.Participant]
	value := price * quantity
	if buyer.Type == Limit {
		reserved := buyer.Price * quantity
		buyerAccount.CashReserved -= reserved
		buyerAccount.CashAvailable += reserved - value
	} else {
		buyerAccount.CashAvailable -= value
	}
	buyerAccount.SharesAvailable += quantity
	if seller.Type == Limit {
		sellerAccount.SharesReserved -= quantity
	} else {
		sellerAccount.SharesAvailable -= quantity
	}
	sellerAccount.CashAvailable += value

	buyer.Filled += quantity
	buyer.Remaining -= quantity
	seller.Filled += quantity
	seller.Remaining -= quantity
	if maker.Remaining == 0 {
		maker.Status = Filled
		e.popMaker(maker)
	} else {
		maker.Status = Partially
	}
	e.sequence++
	trade := Trade{
		ID:            e.nextTradeID,
		BuyOrderID:    buyer.ID,
		SellOrderID:   seller.ID,
		Buyer:         buyer.Participant,
		Seller:        seller.Participant,
		Price:         price,
		Quantity:      quantity,
		AggressorSide: incoming.Side,
		Sequence:      e.sequence,
	}
	e.nextTradeID++
	e.trades = append(e.trades, trade)
	return trade
}

func (e *Engine) rest(order *Order) {
	book := e.bids
	if order.Side == Sell {
		book = e.asks
	}
	book[order.Price] = append(book[order.Price], order.ID)
}

func (e *Engine) popMaker(order *Order) {
	book := e.bids
	if order.Side == Sell {
		book = e.asks
	}
	queue := book[order.Price]
	if len(queue) > 0 && queue[0] == order.ID {
		queue = queue[1:]
	}
	if len(queue) == 0 {
		delete(book, order.Price)
	} else {
		book[order.Price] = queue
	}
}

func (e *Engine) release(order *Order) {
	if order.Type != Limit || order.Remaining == 0 {
		return
	}
	account := e.accounts[order.Participant]
	if order.Side == Buy {
		amount := order.Price * order.Remaining
		account.CashReserved -= amount
		account.CashAvailable += amount
	} else {
		account.SharesReserved -= order.Remaining
		account.SharesAvailable += order.Remaining
	}
}

// Cancel removes an open limit order and releases its remaining reservation.
func (e *Engine) Cancel(participant string, orderID uint64) (Order, error) {
	if _, err := e.takeCommandID(); err != nil {
		return Order{}, err
	}
	order := e.orders[orderID]
	if order == nil {
		return Order{}, ErrOrderNotFound
	}
	if order.Participant != participant {
		return Order{}, ErrNotOrderOwner
	}
	if order.Status != Open && order.Status != Partially {
		return Order{}, ErrOrderNotOpen
	}
	if e.sequence == math.MaxUint64 {
		return Order{}, ErrCapacity
	}
	e.removeFromBook(order)
	e.release(order)
	order.Status = Canceled
	e.sequence++
	return *order, nil
}

func (e *Engine) removeFromBook(order *Order) {
	book := e.bids
	if order.Side == Sell {
		book = e.asks
	}
	queue := book[order.Price]
	for i, id := range queue {
		if id == order.ID {
			queue = append(queue[:i], queue[i+1:]...)
			break
		}
	}
	if len(queue) == 0 {
		delete(book, order.Price)
	} else {
		book[order.Price] = queue
	}
}

func (e *Engine) Account(participant string) (Account, bool) {
	if e == nil {
		return Account{}, false
	}
	account, ok := e.accounts[participant]
	if !ok {
		return Account{}, false
	}
	return *account, true
}

func (e *Engine) Order(orderID uint64) (Order, bool) {
	if e == nil {
		return Order{}, false
	}
	order, ok := e.orders[orderID]
	if !ok {
		return Order{}, false
	}
	return *order, true
}

func (e *Engine) Snapshot() Snapshot {
	snapshot := Snapshot{
		Sequence:      e.sequence,
		NextCommandID: e.nextCommandID,
		NextOrderID:   e.nextOrderID,
		NextTradeID:   e.nextTradeID,
		TotalCash:     e.totalCash,
		TotalShares:   e.totalShares,
		Accounts:      make([]Account, 0, len(e.accounts)),
		Orders:        make([]Order, 0, len(e.orders)),
		Trades:        append(make([]Trade, 0, len(e.trades)), e.trades...),
	}
	for _, account := range e.accounts {
		snapshot.Accounts = append(snapshot.Accounts, *account)
	}
	sort.Slice(snapshot.Accounts, func(i, j int) bool {
		return snapshot.Accounts[i].Participant < snapshot.Accounts[j].Participant
	})
	for _, order := range e.orders {
		snapshot.Orders = append(snapshot.Orders, *order)
	}
	sort.Slice(snapshot.Orders, func(i, j int) bool { return snapshot.Orders[i].ID < snapshot.Orders[j].ID })
	snapshot.Bids = e.levels(e.bids, true)
	snapshot.Asks = e.levels(e.asks, false)
	return snapshot
}

func (e *Engine) levels(book map[int64][]uint64, descending bool) []Level {
	levels := make([]Level, 0, len(book))
	for price, queue := range book {
		level := Level{Price: price}
		for _, id := range queue {
			order := e.orders[id]
			if order != nil && (order.Status == Open || order.Status == Partially) && order.Remaining > 0 {
				level.Quantity += order.Remaining
				level.Orders++
			}
		}
		if level.Orders > 0 {
			levels = append(levels, level)
		}
	}
	sort.Slice(levels, func(i, j int) bool {
		if descending {
			return levels[i].Price > levels[j].Price
		}
		return levels[i].Price < levels[j].Price
	})
	return levels
}

func (e *Engine) CanonicalHash() (string, error) {
	encoded, err := json.Marshal(e.Snapshot())
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func (e *Engine) CheckInvariants() error {
	if e == nil {
		return errors.New("nil engine")
	}
	if e.nextCommandID == 0 || e.nextOrderID == 0 || e.nextTradeID == 0 {
		return errors.New("zero next identifier")
	}
	var totalCash, totalShares int64
	reservedCash := make(map[string]int64, len(e.accounts))
	reservedShares := make(map[string]int64, len(e.accounts))
	for participant, account := range e.accounts {
		if account == nil || participant == "" || account.Participant != participant {
			return fmt.Errorf("invalid account entry %q", participant)
		}
		if account.CashAvailable < 0 || account.CashReserved < 0 || account.SharesAvailable < 0 || account.SharesReserved < 0 {
			return fmt.Errorf("negative balance for %s", participant)
		}
		accountCash, ok := addInt64(account.CashAvailable, account.CashReserved)
		if !ok {
			return fmt.Errorf("cash balance overflow for %s", participant)
		}
		totalCash, ok = addInt64(totalCash, accountCash)
		if !ok {
			return errors.New("cash total overflow")
		}
		accountShares, ok := addInt64(account.SharesAvailable, account.SharesReserved)
		if !ok {
			return fmt.Errorf("share balance overflow for %s", participant)
		}
		totalShares, ok = addInt64(totalShares, accountShares)
		if !ok {
			return errors.New("share total overflow")
		}
	}
	if totalCash != e.totalCash || totalShares != e.totalShares {
		return fmt.Errorf("conservation failed: cash %d/%d shares %d/%d", totalCash, e.totalCash, totalShares, e.totalShares)
	}

	bookCount := make(map[uint64]int)
	if err := e.checkBook(e.bids, Buy, bookCount); err != nil {
		return err
	}
	if err := e.checkBook(e.asks, Sell, bookCount); err != nil {
		return err
	}
	for id, order := range e.orders {
		if order == nil || id == 0 || order.ID != id {
			return fmt.Errorf("invalid order entry %d", id)
		}
		if order.CommandID == 0 || order.CommandID >= e.nextCommandID || order.Sequence == 0 || order.Sequence > e.sequence {
			return fmt.Errorf("invalid identifiers on order %d", id)
		}
		if order.Participant == "" || e.accounts[order.Participant] == nil || order.Side != Buy && order.Side != Sell || order.Type != Limit && order.Type != Market {
			return fmt.Errorf("invalid fields on order %d", id)
		}
		if order.Quantity <= 0 || order.Quantity > MaxQuantity || order.Filled < 0 || order.Remaining < 0 || order.Filled > order.Quantity || order.Remaining != order.Quantity-order.Filled {
			return fmt.Errorf("invalid quantities on order %d", id)
		}
		if order.Type == Limit {
			if _, err := checkedValue(order.Price, order.Quantity); err != nil {
				return fmt.Errorf("invalid limit order %d: %w", id, err)
			}
		} else if order.Price != 0 {
			return fmt.Errorf("market order %d has a price", id)
		}
		active := order.Status == Open || order.Status == Partially
		if active {
			if order.Type != Limit || order.Remaining == 0 || bookCount[id] != 1 {
				return fmt.Errorf("active order %d is not present exactly once in its book", id)
			}
			if order.Status == Open && order.Filled != 0 || order.Status == Partially && order.Filled == 0 {
				return fmt.Errorf("order %d status disagrees with fills", id)
			}
			if order.Side == Buy {
				value := order.Price * order.Remaining
				var ok bool
				reservedCash[order.Participant], ok = addInt64(reservedCash[order.Participant], value)
				if !ok {
					return errors.New("reservation total overflow")
				}
			} else {
				reservedShares[order.Participant] += order.Remaining
			}
		} else {
			if bookCount[id] != 0 {
				return fmt.Errorf("inactive order %d is present in a book", id)
			}
			if order.Status == Filled && order.Remaining != 0 || order.Status == Canceled && order.Remaining == 0 {
				return fmt.Errorf("order %d terminal status disagrees with remainder", id)
			}
			if order.Status != Filled && order.Status != Canceled {
				return fmt.Errorf("order %d has unknown status %q", id, order.Status)
			}
		}
	}
	if uint64(len(e.orders))+1 != e.nextOrderID {
		return fmt.Errorf("next order id %d disagrees with order count %d", e.nextOrderID, len(e.orders))
	}
	for participant, account := range e.accounts {
		if account.CashReserved != reservedCash[participant] || account.SharesReserved != reservedShares[participant] {
			return fmt.Errorf("reservation mismatch for %s: cash %d/%d shares %d/%d", participant, account.CashReserved, reservedCash[participant], account.SharesReserved, reservedShares[participant])
		}
	}

	var previousTradeSequence uint64
	for i, trade := range e.trades {
		if trade.ID != uint64(i+1) || trade.Sequence <= previousTradeSequence || trade.Sequence > e.sequence || trade.Quantity <= 0 || trade.Price <= 0 || trade.Price > MaxPrice {
			return fmt.Errorf("invalid trade %d", trade.ID)
		}
		buy := e.orders[trade.BuyOrderID]
		sell := e.orders[trade.SellOrderID]
		if buy == nil || sell == nil || buy.Side != Buy || sell.Side != Sell || buy.Participant != trade.Buyer || sell.Participant != trade.Seller || trade.Buyer == trade.Seller {
			return fmt.Errorf("trade %d disagrees with its orders", trade.ID)
		}
		if trade.AggressorSide != Buy && trade.AggressorSide != Sell {
			return fmt.Errorf("trade %d has invalid aggressor side", trade.ID)
		}
		maker := sell
		if trade.AggressorSide == Sell {
			maker = buy
		}
		if maker.Price != trade.Price {
			return fmt.Errorf("trade %d did not execute at resting price", trade.ID)
		}
		previousTradeSequence = trade.Sequence
	}
	if uint64(len(e.trades))+1 != e.nextTradeID {
		return fmt.Errorf("next trade id %d disagrees with trade count %d", e.nextTradeID, len(e.trades))
	}
	bids, asks := e.levels(e.bids, true), e.levels(e.asks, false)
	if len(bids) > 0 && len(asks) > 0 && bids[0].Price >= asks[0].Price {
		return fmt.Errorf("crossed book: bid %d ask %d", bids[0].Price, asks[0].Price)
	}
	return nil
}

func (e *Engine) checkBook(book map[int64][]uint64, side Side, count map[uint64]int) error {
	for price, queue := range book {
		if price <= 0 || price > MaxPrice || len(queue) == 0 {
			return fmt.Errorf("invalid %s price level %d", side, price)
		}
		var previousSequence uint64
		for _, id := range queue {
			order := e.orders[id]
			if order == nil || order.Side != side || order.Type != Limit || order.Price != price || order.Status != Open && order.Status != Partially || order.Remaining <= 0 {
				return fmt.Errorf("invalid order %d in %s level %d", id, side, price)
			}
			if order.Sequence <= previousSequence {
				return fmt.Errorf("non-FIFO %s level %d", side, price)
			}
			previousSequence = order.Sequence
			count[id]++
		}
	}
	return nil
}
