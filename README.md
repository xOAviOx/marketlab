# MarketLab

MarketLab is a deterministic stock-exchange simulator and interactive market
laboratory. Trade the fictional asset **NOVA** against seeded strategies, submit
real orders into a price-time-priority book, trigger market shocks, then replay
the exact command stream that produced the result.

Everything in MarketLab is fictional. It has no broker connection, paid API,
real market data, account system, or required AI service.

## Run locally

Requirements: Go 1.23 or newer, Node.js 22 or newer, and npm 10 or newer.

For development, install the web dependencies once:

```sh
npm --prefix web ci
```

Then run these in separate terminals:

```sh
go run ./cmd/marketlab
npm --prefix web run dev
```

The Go server listens on `http://localhost:8080` by default. Vite prints the
development URL and proxies API and WebSocket traffic to that server.

### Build one production service

On Windows PowerShell:

```powershell
./scripts/build.ps1
./dist/marketlab.exe
```

On a Unix-like shell:

```sh
chmod +x scripts/build.sh
./scripts/build.sh
./dist/marketlab
```

Both scripts build the React application, copy it into the Go embed directory,
and produce one executable that serves the frontend, API, and WebSocket feed.
Set `PORT` to change the listener. Containers are optional; `docker build -t
marketlab .` uses the same packaging path.

## Sixty-second demo

1. Open MarketLab and press **Start**. The fixed default seed starts multiple
   market makers, momentum traders, and mean-reversion traders with finite cash
   and shares.
2. Watch the NOVA tape and order-book depth, then submit a small limit or market
   order. The portfolio, open orders, fills, and chart all come back from the
   Go session—not from a browser-side market generator.
3. Trigger **Negative news**. The simulator lowers bot fair-value estimates,
   widens stressed quotes, and submits an actual funded sell order. The last
   price changes only if that order or a later bot order executes.
4. Pause the simulation and open the event inspector. Follow the linked order
   and trade identifiers that explain the move.
5. Enter **Replay**, step through the first events, then scrub forward. Trading
   and scenario controls are read-only while the recorded commands rebuild the
   market.
6. Download the JSON experiment. Import it again to demonstrate the version and
   bounds checks and deterministic replay path.

## Architecture

The short version is:

```text
React terminal ── HTTP commands / WebSocket snapshots ── session actor
                                                           │
                  recorder + logical scheduler + bots + scenarios
                                                           │
                                             engine + account ledger
```

Each visitor gets an isolated, capped in-memory session. One actor goroutine owns
all mutable state in a session; bots run in a stable strategy order on logical
ticks. Wall-clock time controls pacing only, so request timing cannot reorder bot
execution. Slow WebSocket clients receive a newer authoritative snapshot rather
than blocking the engine.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the component diagram,
ownership rules, book data structure, complexity, settlement, and replay
boundaries.

## Matching rules

- NOVA prices and cash use integer cents; quantities use integer shares.
- Limit orders can rest. Market orders consume available liquidity and cancel
  any unfilled remainder.
- Better prices execute first. At one price, the lowest engine sequence executes
  first. An execution always uses the resting order's price.
- Resting buys reserve cash at their limit price. Price improvement is refunded
  during settlement. Resting sells reserve shares. Partial fills and
  cancellations release only the correct remaining reservation.
- Market buys stop before spending more than available cash. Short selling,
  borrowing, and fees are intentionally absent in version one.
- Self-trade prevention cancels the incoming remainder when it reaches the
  participant's own best executable resting order. It does not skip that order
  because doing so would jump the FIFO queue.
- Inputs have explicit price, quantity, body, recording, session, and history
  bounds. Multiplication is checked before settlement.

The invariant suite checks total cash and shares, nonnegative available and
reserved balances, order quantities, uncrossed books, canceled-order behavior,
and deterministic canonical state.

## Strategies and scenarios

**Market makers** refresh two-sided quotes around an estimated fair value. Their
spread, size, interval, inventory skew, and stress response affect the orders
they submit. **Momentum traders** react to executed-price trends over a bounded
lookback. **Mean-reversion traders** trade deviations from their estimated fair
value. Every strategy uses the same validation and finite account ledger as the
human participant, and every explanation is a structured simulator reason—not
LLM output.

The three scenarios change orders and strategy inputs, never the traded price:

- **Negative news** shifts bot fair values, widens maker spreads during stress,
  and creates funded selling pressure before a deterministic scheduled reversal.
- **Liquidity drought** makes makers cancel quotes and temporarily withdraw.
- **Large sell order** submits a market sell from a participant endowed with
  enough NOVA.

If none of those orders can execute, the last trade remains unchanged.

## Recording and replay guarantees

Experiments contain the format and engine versions, seed, initial endowments and
configuration, plus ordered logical-time events with monotonic sequences. Bot
orders are recorded explicitly; replay applies those commands and does **not**
run the bots again. Canonical state excludes network timing and wall-clock
performance data.

Imports reject incompatible versions, malformed or out-of-bounds numeric data,
more than 50,000 events, and files larger than 5 MiB. Histories are bounded in
memory. Seeking currently rebuilds from the initial state in `O(N)` time, which
is deliberate for the bounded first version.

An example from the tested default seed is in
[`examples/seed-424242.marketlab.json`](examples/seed-424242.marketlab.json).

## Test and measurement commands

```sh
# Engine, simulation, transport, and package tests
go test ./internal/... ./cmd/marketlab

# Shared-state race checks
go test -race ./internal/... ./cmd/marketlab

# Matching benchmark, isolated from JSON and UI delivery
go test -run '^$' -bench BenchmarkMatching -benchmem -count=3 ./internal/engine

# React unit tests and production compilation
npm --prefix web test -- --run
npm --prefix web run build

# Browser workflow tests (starts its configured local servers)
npm --prefix web run test:e2e
```

### Measured matching result

Measured locally on 2026-10-01 with Go 1.23.3, Windows/amd64, and an Intel Core
i9-14900K (32 logical CPUs). `BenchmarkMatching` builds 100 resting sell orders
across 100 prices and submits 100 market buys per sample. Three samples were
115,775 ns/op, 120,551 ns/op, and 151,768 ns/op; the median was **120,551 ns per
200-command workload**, with about 90,225 B/op and 438 allocs/op. This benchmark
intentionally excludes JSON serialization, WebSocket delivery, React rendering,
and network latency; it is not presented as end-to-end throughput.

## Optional free deployment

`render.yaml` and the optional Dockerfile can create a Render web service. As of
2026-10-01, Render's published free-service limits say an idle service spins down
after 15 minutes, the next request can take about a minute to wake it, and the
filesystem is ephemeral across spin-downs, restarts, and deploys. MarketLab's
sessions are intentionally in memory and experiments are downloaded by the
browser, so the free demo tolerates those constraints—but it does not promise
permanent uptime or durable server-side recordings. Confirm the provider's
current limits before publishing because free plans change.

No deployment is performed by this repository or its build scripts.

## Version-one limitations

- Sessions and recordings live in process memory; restarting the service clears
  active visitors. Downloaded experiment files are the persistence mechanism.
- Replay seeking rebuilds from the beginning rather than from periodic snapshots.
- The exchange trades one fictional symbol and omits fees, margin, shorting,
  corporate actions, auctions, and advanced order types.
- Bots are intentionally explainable teaching strategies. Their behavior is not
  investment advice and makes no prediction about real markets.
- Snapshots are bounded and intentionally straightforward. The system favors an
  auditable demo over exchange-scale throughput.

## Third-party attribution

Charts use [TradingView Lightweight Charts™](https://www.tradingview.com/) under
Apache License 2.0. The interface retains the required TradingView attribution.
WebSocket transport uses Coder's `websocket` library under the ISC license. See
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) and the copied license texts
under [`licenses/`](licenses/) for redistribution details.
