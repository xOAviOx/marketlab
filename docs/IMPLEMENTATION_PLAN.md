# MarketLab implementation plan

1. Build the deterministic integer matching engine, settlement ledger, invariants, tests, and isolated benchmarks.
2. Add the logical clock, seeded strategies, scenarios, command recording, export/import, and replay.
3. Expose isolated visitor sessions through a standard-library HTTP server and bounded WebSocket snapshots.
4. Build the responsive React terminal against the real server, then verify live trading and replay end to end.
5. Inspect desktop and mobile layouts, run race/unit/browser tests, measure the engine, and document the verified result.

The slices stay runnable: the engine has no transport dependencies, the simulation owns all mutable session state on one goroutine, and the production Go binary embeds the built web application.
