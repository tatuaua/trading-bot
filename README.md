# Trading bot

A small Go trading-bot sandbox that streams live trade data from Finnhub, builds short rolling candles, simulates portfolio trades, and stores both market candles and executed trades in SQLite.

This is currently a paper-trading/demo project. It does not place real broker orders.

## Features

- Streams live trades over Finnhub WebSocket.
- Tracks a demo portfolio across a fixed symbol list.
- Renders a live terminal dashboard with price, price delta, volume delta, quantity, cash, and portfolio P/L.
- Stores candle snapshots in `demo.db`.
- Stores executed simulated trades in a `trades` ledger.
- Uses recent candle history to compare current price/volume movement against rolling averages.

## Strategy

The bot builds candles every `500ms` and compares each symbol's current candle against the recent rolling window.

It buys when:

- current price delta is above the average candle delta
- current candle volume is above the average candle volume

It sells when:

- current price delta is below the average candle delta
- current candle volume is above the average candle volume

Each executed trade uses about `1%` of the current portfolio value as the order size. Buy/sell quantity is calculated from:

```text
quantity = principal / current_price
```

## Data stored

The bot creates two SQLite tables in `demo.db`.

### `candles`

Stores each generated candle:

```text
symbol, timestamp, volume, delta, interval
```

### `trades`

Stores each executed simulated trade:

```text
symbol, side, timestamp, price, quantity, principal, reason, cash_after, portfolio_value_after
```

The `reason` field records the signal values that triggered the trade, such as current delta, average delta, current volume, and average volume.

## Setup

Create a `.env` file with your Finnhub API token:

```env
FINNHUB_TOKEN=your_token_here
```

Install dependencies:

```powershell
go mod download
```

Run the bot:

```powershell
go run .
```

Build it:

```powershell
go build ./...
```

## Inspecting trades

You can inspect the trade ledger with SQLite:

```sql
SELECT
  datetime(timestamp, 'unixepoch') AS time,
  symbol,
  side,
  price,
  quantity,
  principal,
  reason,
  portfolio_value_after
FROM trades
ORDER BY timestamp DESC
LIMIT 20;
```

## Notes

- The portfolio is initialized in code with fixed starting quantities and cash.
- The symbol list comes from the initialized portfolio.
- `demo.db` is local runtime state and can be deleted if you want a fresh run.
