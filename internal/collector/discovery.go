package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/models"
)

type eventsResponse struct {
	Events     []eventDTO `json:"events"`
	Pagination struct {
		Page       int `json:"page"`
		Size       int `json:"size"`
		LastPage   int `json:"lastPage"`
		TotalCount int `json:"totalCount"`
	} `json:"pagination"`
}

type eventDTO struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Engine  string      `json:"engine"`
	Status  string      `json:"status"`
	Markets []marketDTO `json:"markets"`
}

type marketDTO struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Outcome1ID string `json:"outcome1Id"`
	Outcome2ID string `json:"outcome2Id"`
}

// MarketUpdateFunc is called when the list of tracked markets changes.
type MarketUpdateFunc func(markets []models.TrackedMarket)

type Discovery struct {
	relayURL string
	interval time.Duration
	logger   *zap.Logger
	client   *http.Client

	mu        sync.RWMutex
	markets   []models.TrackedMarket
	listeners []MarketUpdateFunc
}

func NewDiscovery(relayURL string, interval time.Duration, logger *zap.Logger) *Discovery {
	return &Discovery{
		relayURL: relayURL,
		interval: interval,
		logger:   logger,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (d *Discovery) OnUpdate(fn MarketUpdateFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, fn)
}

func (d *Discovery) Markets() []models.TrackedMarket {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]models.TrackedMarket, len(d.markets))
	copy(out, d.markets)
	return out
}

func (d *Discovery) Run(ctx context.Context) {
	d.logger.Info("starting market discovery", zap.Duration("interval", d.interval))

	// Initial fetch
	d.discover(ctx)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("market discovery stopped")
			return
		case <-ticker.C:
			d.discover(ctx)
		}
	}
}

func (d *Discovery) discover(ctx context.Context) {
	markets, err := d.fetchAllCLOBMarkets(ctx)
	if err != nil {
		d.logger.Error("failed to discover markets", zap.Error(err))
		return
	}

	if len(markets) == 0 {
		d.logger.Warn("discovery returned zero markets, keeping previous list")
		return
	}

	d.mu.Lock()
	d.markets = markets
	listeners := make([]MarketUpdateFunc, len(d.listeners))
	copy(listeners, d.listeners)
	d.mu.Unlock()

	d.logger.Info("discovered CLOB markets", zap.Int("count", len(markets)))

	for _, fn := range listeners {
		fn(markets)
	}
}

func (d *Discovery) fetchAllCLOBMarkets(ctx context.Context) ([]models.TrackedMarket, error) {
	var allMarkets []models.TrackedMarket
	page := 1

	for {
		resp, err := d.fetchEventsPage(ctx, page)
		if err != nil {
			return nil, err
		}

		for _, event := range resp.Events {
			if event.Engine != "CLOB" {
				continue
			}
			for _, market := range event.Markets {
				if market.Status != "open" {
					continue
				}
				allMarkets = append(allMarkets, models.TrackedMarket{
					MarketID:   market.ID,
					EventID:    event.ID,
					EventTitle: event.Title,
					Outcome1ID: market.Outcome1ID,
					Outcome2ID: market.Outcome2ID,
				})
			}
		}

		if page >= resp.Pagination.LastPage || page >= 100 {
			break
		}
		page++
	}

	return allMarkets, nil
}

func (d *Discovery) fetchEventsPage(ctx context.Context, page int) (*eventsResponse, error) {
	url := fmt.Sprintf("%s/v1/pm/events?status=open&page=%d&size=50", d.relayURL, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result eventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}
