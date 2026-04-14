package models

// TrackedMarket is an in-memory representation of a discovered CLOB market.
// Not persisted — rebuilt each discovery cycle.
type TrackedMarket struct {
	MarketID   string
	EventID    string
	EventTitle string
	Outcome1ID string
	Outcome2ID string
}
