# MarketLab architecture

```text
 Browser (React + Zustand + Lightweight Charts)
     │  commands over HTTP / snapshots over WebSocket
     ▼
 Session manager ── capped, isolated visitor sessions + inactivity cleanup
     │
     ├── Session actor A ── one serialized owner goroutine
     │      ├── logical clock and deterministic scheduler
     │      ├── market-maker, momentum, and mean-reversion strategies
     │      ├── scenario controller and scheduled reversals
     │      ├── versioned command/event recorder and replay cursor
     │      └── matching engine + account ledger
     ├── Session actor B
     └── …
```

The engine has no HTTP, browser, goroutine, PRNG, or wall-clock dependency. A
session actor owns its engine and processes commands in one order. Bot turns are
scheduled by logical time and strategy order, so a network race cannot reorder
bot decisions. Wall time only decides when the actor advances another logical
step and when a snapshot is offered to connected browsers.

## Matching and settlement

NOVA trades in integer cents and integer shares. Each side of the book is a map
from price to a FIFO queue of engine order identifiers. The bounded reference
implementation sorts active price keys when finding the best match, making a
match `O(P log P)` in the number of active price levels; consuming the head of a
level is amortized `O(1)`. The sequence assigned by the engine, not map iteration
order or wall time, establishes FIFO priority.

An incoming order executes at the resting order's price. A limit buy reserves
its limit price multiplied by its remaining quantity; settlement refunds any
price improvement. A limit sell reserves shares. Cancellation and any canceled
remainder release those reservations. Market orders never rest. They consume
only available liquidity and a market buy also stops before exceeding available
cash.

Self-trade prevention is deterministic: when an incoming order reaches the
participant's own order at the best executable price, MarketLab cancels the
incoming remainder. It does not skip that resting order, which would let the
aggressor reorder price-time priority around its own liquidity.

## Recording and replay

The recording contains the seed, initial configuration and endowments, engine
and format versions, and the ordered logical-time command stream. Bot-generated
orders are explicit recorded commands. Replay applies those commands and does
not run strategies a second time. Seeking is intentionally simple in version
one: rebuild the bounded state from the initial endowments through the selected
command. Wall-clock pacing and network telemetry are excluded from canonical
state hashes.

## Delivery boundaries

The server publishes authoritative, bounded snapshots no faster than the UI
cadence. Each WebSocket subscriber has a bounded nonblocking queue, so a slow
client drops superseded snapshots instead of blocking its session actor. The
built frontend is embedded in the Go executable; Vite's development server is
only a local convenience.

