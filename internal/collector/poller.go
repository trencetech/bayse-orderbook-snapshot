package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/models"
)

type booksResponse []orderbookPayload

type Poller struct {
	relayURL string
	interval time.Duration
	logger   *zap.Logger
	client   *http.Client
	writeCh  chan<- models.OrderbookSnapshot

	mu      sync.RWMutex
	markets []models.TrackedMarket
}

func NewPoller(relayURL string, interval time.Duration, logger *zap.Logger, writeCh chan<- models.OrderbookSnapshot) *Poller {
	return &Poller{
		relayURL: relayURL,
		interval: interval,
		logger:   logger,
		client:   &http.Client{Timeout: 15 * time.Second},
		writeCh:  writeCh,
	}
}

func (p *Poller) UpdateMarkets(markets []models.TrackedMarket) {
	p.mu.Lock()
	p.markets = markets
	p.mu.Unlock()
}

func (p *Poller) Run(ctx context.Context) {
	p.logger.Info("starting REST poller", zap.Duration("interval", p.interval))

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("REST poller stopped")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	p.mu.RLock()
	markets := make([]models.TrackedMarket, len(p.markets))
	copy(markets, p.markets)
	p.mu.RUnlock()

	if len(markets) == 0 {
		return
	}

	// Collect all outcome IDs
	var outcomeIDs []string
	for _, m := range markets {
		outcomeIDs = append(outcomeIDs, m.Outcome1ID, m.Outcome2ID)
	}

	batchSize := 20
	for i := 0; i < len(outcomeIDs); i += batchSize {
		end := i + batchSize
		if end > len(outcomeIDs) {
			end = len(outcomeIDs)
		}
		p.fetchBatch(ctx, outcomeIDs[i:end])
	}
}

func (p *Poller) fetchBatch(ctx context.Context, outcomeIDs []string) {
	params := neturl.Values{}
	params.Set("depth", "20")
	for _, id := range outcomeIDs {
		params.Add("outcomeId[]", id)
	}

	reqURL := fmt.Sprintf("%s/v1/pm/books?%s", p.relayURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		p.logger.Error("failed to create books request", zap.Error(err))
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Error("failed to fetch books", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		p.logger.Error("unexpected books response", zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
		return
	}

	var books booksResponse
	if err := json.NewDecoder(resp.Body).Decode(&books); err != nil {
		p.logger.Error("failed to decode books response", zap.Error(err))
		return
	}

	for _, ob := range books {
		snapshot := models.NewSnapshot(
			ob.MarketID,
			ob.OutcomeID,
			"rest",
			ob.Bids,
			ob.Asks,
			ob.LastTradedPrice,
			ob.LastTradedSide,
		)

		select {
		case p.writeCh <- snapshot:
		default:
			p.logger.Warn("write channel full, dropping REST snapshot")
		}
	}

	p.logger.Debug("polled orderbooks", zap.Int("count", len(books)))
}
