package models

import (
	"time"

	"github.com/uptrace/bun"
)

type PriceLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Total    float64 `json:"total"`
}

type OrderbookSnapshot struct {
	bun.BaseModel `bun:"table:orderbook_snapshots"`

	Time            time.Time    `bun:"time,notnull,type:timestamptz"        json:"time"`
	MarketID        string       `bun:"market_id,notnull,type:uuid"          json:"marketId"`
	OutcomeID       string       `bun:"outcome_id,notnull,type:uuid"         json:"outcomeId"`
	Source          string       `bun:"source,notnull,type:varchar(10)"      json:"source"`
	Bids            []PriceLevel `bun:"bids,notnull,type:jsonb"              json:"bids"`
	Asks            []PriceLevel `bun:"asks,notnull,type:jsonb"              json:"asks"`
	BidCount        int          `bun:"bid_count,notnull"                    json:"bidCount"`
	AskCount        int          `bun:"ask_count,notnull"                    json:"askCount"`
	BestBid         *float64     `bun:"best_bid,type:numeric(10,4)"          json:"bestBid"`
	BestAsk         *float64     `bun:"best_ask,type:numeric(10,4)"          json:"bestAsk"`
	Spread          *float64     `bun:"spread,type:numeric(10,4)"            json:"spread"`
	TotalBidDepth   float64      `bun:"total_bid_depth,notnull"              json:"totalBidDepth"`
	TotalAskDepth   float64      `bun:"total_ask_depth,notnull"              json:"totalAskDepth"`
	LastTradedPrice *float64     `bun:"last_traded_price,type:numeric(10,4)" json:"lastTradedPrice"`
	LastTradedSide  *string      `bun:"last_traded_side,type:varchar(4)"     json:"lastTradedSide"`
}

// NewSnapshot builds a snapshot with pre-computed derived fields.
func NewSnapshot(
	marketID, outcomeID, source string,
	bids, asks []PriceLevel,
	lastTradedPrice *float64,
	lastTradedSide *string,
) OrderbookSnapshot {
	s := OrderbookSnapshot{
		Time:            time.Now().UTC(),
		MarketID:        marketID,
		OutcomeID:       outcomeID,
		Source:          source,
		Bids:            bids,
		Asks:            asks,
		BidCount:        len(bids),
		AskCount:        len(asks),
		LastTradedPrice: lastTradedPrice,
		LastTradedSide:  lastTradedSide,
	}

	for _, b := range bids {
		s.TotalBidDepth += b.Quantity
	}
	for _, a := range asks {
		s.TotalAskDepth += a.Quantity
	}

	if len(bids) > 0 {
		s.BestBid = &bids[0].Price
	}
	if len(asks) > 0 {
		s.BestAsk = &asks[0].Price
	}
	if s.BestBid != nil && s.BestAsk != nil {
		spread := *s.BestAsk - *s.BestBid
		s.Spread = &spread
	}

	return s
}
