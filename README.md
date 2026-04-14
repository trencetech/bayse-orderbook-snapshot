# bayse-orderbook-snapshot

An example application that captures orderbook snapshots from [Bayse](https://bayse.markets) prediction markets and stores them in a time-series database for historical analysis.

## Stack

- **Go** — application runtime
- **TimescaleDB** (PostgreSQL extension) — time-series storage for orderbook snapshots

## How it works

The application connects to the Bayse public API to discover active CLOB markets, then collects orderbook data through two channels:

- **WebSocket** — subscribes to real-time orderbook updates
- **REST polling** — periodically fetches full orderbook snapshots as a gap-fill

Snapshots are buffered and bulk-inserted into a TimescaleDB hypertable. A query API exposes historical snapshots, latest state, and time-bucketed spread/depth statistics.

## Requirements

- Go 1.25+
- A PostgreSQL instance with the [TimescaleDB extension](https://docs.timescale.com/) enabled

## Setup

1. **Install dependencies**

```sh
make setup
```

2. **Configure environment**

Copy the example env file and fill in your database credentials:

```sh
cp .env.example .env
```

3. **Run migrations**

Initialize the migration table, then run pending migrations:

```sh
make migrate-init
make migrate
```

4. **Run the application**

```sh
make run
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/version` | Build version |
| `GET` | `/v1/snapshots` | Query historical snapshots by market and time range |
| `GET` | `/v1/snapshots/latest` | Get the latest snapshot for a market |
| `GET` | `/v1/snapshots/stats` | Time-bucketed spread and depth statistics |
