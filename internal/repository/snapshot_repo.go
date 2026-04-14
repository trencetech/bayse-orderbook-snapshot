package repository

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/models"
)

type SpreadStatBucket struct {
	Bucket      time.Time `json:"bucket"`
	OutcomeID   string    `json:"outcomeId"`
	AvgSpread   *float64  `json:"avgSpread"`
	AvgBidDepth float64   `json:"avgBidDepth"`
	AvgAskDepth float64   `json:"avgAskDepth"`
	MinBid      *float64  `json:"minBid"`
	MaxAsk      *float64  `json:"maxAsk"`
	SampleCount int64     `json:"sampleCount"`
}

type SnapshotRepository struct {
	db *bun.DB
}

func NewSnapshotRepository(db *bun.DB) *SnapshotRepository {
	return &SnapshotRepository{db: db}
}

func (r *SnapshotRepository) BulkInsert(ctx context.Context, snapshots []models.OrderbookSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	_, err := r.db.NewInsert().Model(&snapshots).Exec(ctx)
	return err
}

func (r *SnapshotRepository) FindByMarket(
	ctx context.Context,
	marketID string,
	outcomeID *string,
	from, to time.Time,
	limit int,
	source *string,
) ([]models.OrderbookSnapshot, error) {
	var snapshots []models.OrderbookSnapshot

	q := r.db.NewSelect().
		Model(&snapshots).
		Where("market_id = ?", marketID).
		Where("time >= ?", from).
		Where("time <= ?", to).
		OrderExpr("time DESC").
		Limit(limit)

	if outcomeID != nil {
		q = q.Where("outcome_id = ?", *outcomeID)
	}
	if source != nil {
		q = q.Where("source = ?", *source)
	}

	err := q.Scan(ctx)
	return snapshots, err
}

func (r *SnapshotRepository) FindLatest(ctx context.Context, marketID string) ([]models.OrderbookSnapshot, error) {
	var snapshots []models.OrderbookSnapshot

	err := r.db.NewSelect().
		Model(&snapshots).
		Where("market_id = ?", marketID).
		Where("time = (SELECT MAX(time) FROM orderbook_snapshots s2 WHERE s2.market_id = ?0 AND s2.outcome_id = \"orderbook_snapshot\".outcome_id)", marketID).
		Scan(ctx)

	return snapshots, err
}

func (r *SnapshotRepository) GetSpreadStats(
	ctx context.Context,
	marketID string,
	outcomeID *string,
	from, to time.Time,
	interval string,
) ([]SpreadStatBucket, error) {
	var buckets []SpreadStatBucket

	args := []interface{}{interval, marketID}
	outcomeFilter := ""
	if outcomeID != nil {
		outcomeFilter = "AND outcome_id = ?"
		args = append(args, *outcomeID)
	}
	args = append(args, from, to)

	q := r.db.NewRaw(`
		SELECT
			time_bucket(?, time) AS bucket,
			outcome_id,
			AVG(spread) AS avg_spread,
			AVG(total_bid_depth) AS avg_bid_depth,
			AVG(total_ask_depth) AS avg_ask_depth,
			MIN(best_bid) AS min_bid,
			MAX(best_ask) AS max_ask,
			COUNT(*) AS sample_count
		FROM orderbook_snapshots
		WHERE market_id = ?
		  `+outcomeFilter+`
		  AND time >= ?
		  AND time <= ?
		GROUP BY bucket, outcome_id
		ORDER BY bucket DESC
	`, args...)

	err := r.db.NewSelect().TableExpr("(?) AS stats", q).Scan(ctx, &buckets)
	return buckets, err
}
