package collector

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/models"
)

const (
	wsReadTimeout  = 90 * time.Second
	wsPingInterval = 30 * time.Second
	wsMaxMsgSize   = 1 << 20 // 1 MB
)

type wsMessage struct {
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	ClientID  string          `json:"clientId"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

type orderbookUpdateData struct {
	Orderbook orderbookPayload `json:"orderbook"`
}

type orderbookPayload struct {
	MarketID        string              `json:"marketId"`
	OutcomeID       string              `json:"outcomeId"`
	Timestamp       string              `json:"timestamp"`
	Bids            []models.PriceLevel `json:"bids"`
	Asks            []models.PriceLevel `json:"asks"`
	LastTradedPrice *float64            `json:"lastTradedPrice"`
	LastTradedSide  *string             `json:"lastTradedSide"`
}

// connWriter serializes all writes to a websocket.Conn.
// gorilla/websocket only supports one concurrent writer.
type connWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (cw *connWriter) WriteJSON(v any) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.conn.WriteJSON(v)
}

func (cw *connWriter) WritePing() error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.conn.WriteMessage(websocket.PingMessage, nil)
}

type WSCollector struct {
	wsURL   string
	logger  *zap.Logger
	writeCh chan<- models.OrderbookSnapshot

	mu        sync.RWMutex
	markets   []models.TrackedMarket
	ready     chan struct{} // closed after first market update
	readyOnce sync.Once
}

func NewWSCollector(wsURL string, logger *zap.Logger, writeCh chan<- models.OrderbookSnapshot) *WSCollector {
	return &WSCollector{
		wsURL:   wsURL,
		logger:  logger,
		writeCh: writeCh,
		ready:   make(chan struct{}),
	}
}

func (w *WSCollector) UpdateMarkets(markets []models.TrackedMarket) {
	w.mu.Lock()
	w.markets = markets
	w.mu.Unlock()
	w.readyOnce.Do(func() { close(w.ready) })
}

func (w *WSCollector) Run(ctx context.Context) {
	w.logger.Info("starting websocket collector, waiting for market discovery...")

	// Wait for first discovery result before connecting
	select {
	case <-ctx.Done():
		w.logger.Info("websocket collector stopped")
		return
	case <-w.ready:
	}

	w.logger.Info("market discovery ready, starting websocket connections")

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("websocket collector stopped")
			return
		default:
		}

		connStart := time.Now()
		err := w.connect(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Error("websocket connection error", zap.Error(err))
		}

		// Reset backoff if the connection was alive for a meaningful period
		if time.Since(connStart) > 30*time.Second {
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (w *WSCollector) connect(ctx context.Context) error {
	w.logger.Info("connecting to websocket", zap.String("url", w.wsURL))

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Close connection when context is cancelled to unblock ReadMessage
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// Limit max message size to prevent OOM from oversized messages
	conn.SetReadLimit(wsMaxMsgSize)

	// Set up keepalive: refresh read deadline on every pong
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	// Serialize all writes through connWriter
	cw := &connWriter{conn: conn}

	// Periodic ping sender
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := cw.WritePing(); err != nil {
					return
				}
			}
		}
	}()

	// Read the connected message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return err
	}

	var connMsg wsMessage
	if err := json.Unmarshal(msg, &connMsg); err == nil {
		w.logger.Info("websocket connected", zap.String("clientId", connMsg.ClientID))
	}

	// Subscribe to current markets
	w.subscribe(cw)

	// Handle messages
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		// Refresh read deadline on every message received
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))

		// Messages can be batched with \n separators
		parts := strings.Split(string(msg), "\n")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			w.handleMessage([]byte(part))
		}
	}
}

func (w *WSCollector) subscribe(cw *connWriter) {
	w.mu.RLock()
	markets := make([]models.TrackedMarket, len(w.markets))
	copy(markets, w.markets)
	w.mu.RUnlock()

	if len(markets) == 0 {
		w.logger.Warn("no markets to subscribe to")
		return
	}

	// Subscribe in batches of 10 (Bayse limit)
	batchSize := 10
	for i := 0; i < len(markets); i += batchSize {
		end := i + batchSize
		if end > len(markets) {
			end = len(markets)
		}

		var marketIDs []string
		for _, m := range markets[i:end] {
			marketIDs = append(marketIDs, m.MarketID)
		}

		sub := map[string]any{
			"type":      "subscribe",
			"channel":   "orderbook",
			"marketIds": marketIDs,
			"currency":  "USD",
		}

		if err := cw.WriteJSON(sub); err != nil {
			w.logger.Error("failed to subscribe", zap.Error(err))
			return
		}

		w.logger.Info("subscribed to orderbook", zap.Int("markets", len(marketIDs)))
	}
}

func (w *WSCollector) handleMessage(data []byte) {
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		w.logger.Error("failed to unmarshal message", zap.Error(err))
		return
	}

	switch msg.Type {
	case "orderbook_update":
		w.handleOrderbookUpdate(msg.Data)
	case "pong", "subscribed", "unsubscribed":
		// Expected control messages
	case "error":
		w.logger.Warn("websocket error message", zap.String("data", string(msg.Data)))
	}
}

func (w *WSCollector) handleOrderbookUpdate(data json.RawMessage) {
	var update orderbookUpdateData
	if err := json.Unmarshal(data, &update); err != nil {
		w.logger.Error("failed to unmarshal orderbook update", zap.Error(err))
		return
	}

	ob := update.Orderbook
	snapshot := models.NewSnapshot(
		ob.MarketID,
		ob.OutcomeID,
		"ws",
		ob.Bids,
		ob.Asks,
		ob.LastTradedPrice,
		ob.LastTradedSide,
	)

	select {
	case w.writeCh <- snapshot:
	default:
		w.logger.Warn("write channel full, dropping snapshot")
	}
}
