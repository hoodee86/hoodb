package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shauntso/hoodb/internal/service"
)

// Handler HTTP API处理器
type Handler struct {
	kv *service.KVService
}

// NewHandler 创建新的HTTP处理器
func NewHandler(kv *service.KVService) *Handler {
	return &Handler{
		kv: kv,
	}
}

// SetupRoutes 设置路由
func (h *Handler) SetupRoutes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// CORS中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// KV操作接口
	r.PUT("/kv/:key", h.PutKey)
	r.GET("/kv/:key", h.GetKey)
	r.DELETE("/kv/:key", h.DeleteKey)

	// 批量操作接口
	r.POST("/kv/batch", h.BatchPut)

	// 状态接口
	r.GET("/status", h.GetStatus)
	r.GET("/cluster/stats", h.GetStatus)
	r.GET("/health", h.HealthCheck)

	return r
}

// retryOnLeaderChange 重试机制：在Leader切换时自动重试
func (h *Handler) retryOnLeaderChange(op func() error) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := op()
		if err == nil {
			return nil
		}
		errStr := err.Error()
		// 如果是 "not leader" 错误，等待一小段时间后重试
		if strings.Contains(errStr, "not leader") || strings.Contains(errStr, "leadership lost") {
			time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
			continue
		}
		// 如果是 Raft 内部超时，也重试
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "leadership transfer") {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
			continue
		}
		return err
	}
	return fmt.Errorf("operation failed after %d retries", maxRetries)
}

// PutKeyRequest PUT请求体
type PutKeyRequest struct {
	Value string `json:"value" binding:"required"`
}

// BatchPutRequest 批量PUT请求体
type BatchPutRequest struct {
	Items map[string]string `json:"items" binding:"required"`
}

// PutKey 设置key-value (带重试)
func (h *Handler) PutKey(c *gin.Context) {
	key := c.Param("key")

	var req PutKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// 如果不是Leader，转发到Leader
	if !h.kv.IsLeader() {
		leader := h.kv.GetLeader()
		if leader == "" {
			// 无Leader时重试等待
			for i := 0; i < 5; i++ {
				time.Sleep(200 * time.Millisecond)
				leader = h.kv.GetLeader()
				if leader != "" {
					break
				}
			}
			if leader == "" {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error": "no leader available, cluster may be electing",
				})
				return
			}
		}
		c.JSON(http.StatusTemporaryRedirect, gin.H{
			"error":  "not leader",
			"leader": leader,
		})
		return
	}

	// 使用批处理写入 + 重试
	err := h.retryOnLeaderChange(func() error {
		return h.kv.SetBatched(key, req.Value)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"key":     key,
		"value":   req.Value,
	})
}

// BatchPut 批量设置key-value (单次Raft提交)
func (h *Handler) BatchPut(c *gin.Context) {
	// 先尝试绑定JSON
	var req BatchPutRequest

	// 手动读取body以便更好的错误处理
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// 如果标准绑定失败，尝试重新绑定
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body, expected {\"items\": {\"key1\": \"val1\", ...}}",
		})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "items cannot be empty",
		})
		return
	}

	if !h.kv.IsLeader() {
		c.JSON(http.StatusTemporaryRedirect, gin.H{
			"error":  "not leader",
			"leader": h.kv.GetLeader(),
		})
		return
	}

	err = h.retryOnLeaderChange(func() error {
		return h.kv.BatchSet(req.Items)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"count":   len(req.Items),
	})
}

// GetKey 获取key的值
func (h *Handler) GetKey(c *gin.Context) {
	key := c.Param("key")

	value, err := h.kv.Get(key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":   key,
		"value": value,
	})
}

// DeleteKey 删除key (带重试)
func (h *Handler) DeleteKey(c *gin.Context) {
	key := c.Param("key")

	if !h.kv.IsLeader() {
		c.JSON(http.StatusTemporaryRedirect, gin.H{
			"error":  "not leader",
			"leader": h.kv.GetLeader(),
		})
		return
	}

	err := h.retryOnLeaderChange(func() error {
		return h.kv.Delete(key)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "deleted",
		"key":     key,
	})
}

// GetStatus 获取节点状态
func (h *Handler) GetStatus(c *gin.Context) {
	stats := h.kv.GetStats()
	c.JSON(http.StatusOK, stats)
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}
