package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/version"
)

type SystemHandler struct{}

func (h *SystemHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SystemHandler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"commit": version.CommitSHA})
}
