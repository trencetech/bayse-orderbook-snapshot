package http

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/config"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/handlers"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/middleware"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/repository"
)

type RouterResult struct {
	Engine   *gin.Engine
	Shutdown func()
}

func NewRouter(repo *repository.SnapshotRepository, logger *zap.Logger, cfg *config.Config) *RouterResult {
	if cfg.Env != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Don't trust any proxies by default — override via config if behind a LB
	router.SetTrustedProxies(nil)

	// Security headers
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	})

	router.Use(middleware.TraceID())
	router.Use(middleware.RequestLogger(logger))
	router.Use(gin.Recovery())

	ipLimiter := middleware.NewPerKeyLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)

	systemHandler := &handlers.SystemHandler{}
	snapshotHandler := handlers.NewSnapshotHandler(repo, logger)

	router.GET("/health", systemHandler.Health)
	router.GET("/version", systemHandler.Version)

	v1 := router.Group("/v1", middleware.IPRateLimit(ipLimiter))
	{
		v1.GET("/snapshots", snapshotHandler.List)
		v1.GET("/snapshots/latest", snapshotHandler.Latest)
		v1.GET("/snapshots/stats", snapshotHandler.Stats)
	}

	return &RouterResult{
		Engine:   router,
		Shutdown: func() { ipLimiter.Close() },
	}
}
