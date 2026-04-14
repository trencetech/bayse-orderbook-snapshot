package handlers

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/httperror"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/repository"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type SnapshotHandler struct {
	repo   *repository.SnapshotRepository
	logger *zap.Logger
}

func NewSnapshotHandler(repo *repository.SnapshotRepository, logger *zap.Logger) *SnapshotHandler {
	return &SnapshotHandler{repo: repo, logger: logger}
}

func (h *SnapshotHandler) List(c *gin.Context) {
	marketID := c.Query("marketId")
	if !isValidUUID(marketID) {
		httperror.BadRequest(c, "marketId is required and must be a valid UUID")
		return
	}

	var outcomeID *string
	if oid := c.Query("outcomeId"); oid != "" {
		if !isValidUUID(oid) {
			httperror.BadRequest(c, "outcomeId must be a valid UUID")
			return
		}
		outcomeID = &oid
	}

	var source *string
	if s := c.Query("source"); s != "" {
		if s != "ws" && s != "rest" {
			httperror.BadRequest(c, "source must be 'ws' or 'rest'")
			return
		}
		source = &s
	}

	now := time.Now().UTC()
	from := now.Add(-time.Hour)
	to := now

	if f := c.Query("from"); f != "" {
		parsed, err := time.Parse(time.RFC3339, f)
		if err != nil {
			httperror.BadRequest(c, "from must be a valid RFC3339 timestamp")
			return
		}
		from = parsed
	}
	if t := c.Query("to"); t != "" {
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			httperror.BadRequest(c, "to must be a valid RFC3339 timestamp")
			return
		}
		to = parsed
	}

	maxListRange := 7 * 24 * time.Hour
	if to.Sub(from) > maxListRange {
		httperror.BadRequest(c, "time range cannot exceed 7 days")
		return
	}

	limit := 100
	if l := c.Query("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 || parsed > 1000 {
			httperror.BadRequest(c, "limit must be between 1 and 1000")
			return
		}
		limit = parsed
	}

	snapshots, err := h.repo.FindByMarket(c.Request.Context(), marketID, outcomeID, from, to, limit, source)
	if err != nil {
		h.logger.Error("failed to query snapshots", zap.Error(err))
		httperror.InternalError(c, "failed to query snapshots")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshots": snapshots,
		"count":     len(snapshots),
	})
}

func (h *SnapshotHandler) Latest(c *gin.Context) {
	marketID := c.Query("marketId")
	if !isValidUUID(marketID) {
		httperror.BadRequest(c, "marketId is required and must be a valid UUID")
		return
	}

	snapshots, err := h.repo.FindLatest(c.Request.Context(), marketID)
	if err != nil {
		h.logger.Error("failed to query latest snapshots", zap.Error(err))
		httperror.InternalError(c, "failed to query latest snapshots")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshots": snapshots,
		"count":     len(snapshots),
	})
}

func (h *SnapshotHandler) Stats(c *gin.Context) {
	marketID := c.Query("marketId")
	if !isValidUUID(marketID) {
		httperror.BadRequest(c, "marketId is required and must be a valid UUID")
		return
	}

	var outcomeID *string
	if oid := c.Query("outcomeId"); oid != "" {
		if !isValidUUID(oid) {
			httperror.BadRequest(c, "outcomeId must be a valid UUID")
			return
		}
		outcomeID = &oid
	}

	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if f := c.Query("from"); f != "" {
		parsed, err := time.Parse(time.RFC3339, f)
		if err != nil {
			httperror.BadRequest(c, "from must be a valid RFC3339 timestamp")
			return
		}
		from = parsed
	}
	if t := c.Query("to"); t != "" {
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			httperror.BadRequest(c, "to must be a valid RFC3339 timestamp")
			return
		}
		to = parsed
	}

	maxStatsRange := 30 * 24 * time.Hour
	if to.Sub(from) > maxStatsRange {
		httperror.BadRequest(c, "time range for stats cannot exceed 30 days")
		return
	}

	allowedIntervals := map[string]bool{
		"1 minute": true, "5 minutes": true, "15 minutes": true,
		"30 minutes": true, "1 hour": true, "6 hours": true, "1 day": true,
	}
	interval := "5 minutes"
	if i := c.Query("interval"); i != "" {
		if !allowedIntervals[i] {
			httperror.BadRequest(c, "interval must be one of: 1 minute, 5 minutes, 15 minutes, 30 minutes, 1 hour, 6 hours, 1 day")
			return
		}
		interval = i
	}

	buckets, err := h.repo.GetSpreadStats(c.Request.Context(), marketID, outcomeID, from, to, interval)
	if err != nil {
		h.logger.Error("failed to query stats", zap.Error(err))
		httperror.InternalError(c, "failed to query stats")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": buckets,
		"count": len(buckets),
	})
}

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(s)
}
