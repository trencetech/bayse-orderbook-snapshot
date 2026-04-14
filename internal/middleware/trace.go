package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" || len(traceID) > 64 {
			traceID = uuid.NewString()
		}
		c.Set("traceId", traceID)
		c.Header("X-Trace-ID", traceID)
		c.Next()
	}
}
