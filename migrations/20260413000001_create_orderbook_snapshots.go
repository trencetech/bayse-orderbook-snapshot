package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			_, err := tx.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS timescaledb`)
			if err != nil {
				return fmt.Errorf("failed to create timescaledb extension: %w", err)
			}

			_, err = tx.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS orderbook_snapshots (
					time              TIMESTAMPTZ    NOT NULL,
					market_id         UUID           NOT NULL,
					outcome_id        UUID           NOT NULL,
					source            VARCHAR(10)    NOT NULL DEFAULT 'ws',
					bids              JSONB          NOT NULL DEFAULT '[]',
					asks              JSONB          NOT NULL DEFAULT '[]',
					bid_count         INT            NOT NULL DEFAULT 0,
					ask_count         INT            NOT NULL DEFAULT 0,
					best_bid          NUMERIC(10,4),
					best_ask          NUMERIC(10,4),
					spread            NUMERIC(10,4),
					total_bid_depth   NUMERIC(14,4)  NOT NULL DEFAULT 0,
					total_ask_depth   NUMERIC(14,4)  NOT NULL DEFAULT 0,
					last_traded_price NUMERIC(10,4),
					last_traded_side  VARCHAR(4)
				)
			`)
			if err != nil {
				return fmt.Errorf("failed to create orderbook_snapshots table: %w", err)
			}

			_, err = tx.ExecContext(ctx, `SELECT create_hypertable('orderbook_snapshots', 'time', if_not_exists => TRUE)`)
			if err != nil {
				return fmt.Errorf("failed to create hypertable: %w", err)
			}

			_, err = tx.ExecContext(ctx, `
				CREATE INDEX IF NOT EXISTS idx_snapshots_market_time
				ON orderbook_snapshots (market_id, time DESC)
			`)
			if err != nil {
				return fmt.Errorf("failed to create market_time index: %w", err)
			}

			_, err = tx.ExecContext(ctx, `
				CREATE INDEX IF NOT EXISTS idx_snapshots_outcome_time
				ON orderbook_snapshots (outcome_id, time DESC)
			`)
			if err != nil {
				return fmt.Errorf("failed to create outcome_time index: %w", err)
			}

			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS orderbook_snapshots`)
		return err
	}

	Migrations.MustRegister(up, down)
}
