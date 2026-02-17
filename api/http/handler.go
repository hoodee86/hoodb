package http

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shauntso/hoodb/internal/config"
	"github.com/shauntso/hoodb/internal/service"
)

// Handler HTTP API处理器
type Handler struct {
	kv     *service.KVService
	config *config.Config
}

// NewHandler 创建新的HTTP处理器
func NewHandler(kv *service.KVService, cfg *config.Config) *Handler {
	return &Handler{
		kv:     kv,
		config: cfg,
	}
}

// SetupRoutes 设置路由
func (h *Handler) SetupRoutes() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(corsMiddleware())

	// ----- /api/v1 路由组 -----
	v1 := r.Group("/api/v1")
	v1.Use(h.authMiddleware())
	{
		// KV 操作
		v1.PUT("/kv/:key", h.PutKey)
		v1.GET("/kv/:key", h.GetKey)
		v1.DELETE("/kv/:key", h.DeleteKey)
		v1.POST("/kv/batch", h.BatchPut)
		v1.GET("/kv", h.ListKeys) // 前缀扫描 / Key 列表

		// 集群管理
		v1.POST("/cluster/add", h.AddNode)
		v1.POST("/cluster/remove", h.RemoveNode)
		v1.GET("/cluster/config", h.GetClusterConfig)
		v1.GET("/cluster/stats", h.GetStatus)

		// 基准测试
		v1.POST("/benchmark", h.RunBenchmark)
	}

	// ----- 不带版本号的兼容路由（保持向后兼容） -----
	compat := r.Group("")
	compat.Use(h.authMiddleware())
	{
		compat.PUT("/kv/:key", h.PutKey)
		compat.GET("/kv/:key", h.GetKey)
		compat.DELETE("/kv/:key", h.DeleteKey)
		compat.POST("/kv/batch", h.BatchPut)
		compat.GET("/kv", h.ListKeys)
		compat.POST("/benchmark", h.RunBenchmark)
		compat.POST("/cluster/add", h.AddNode)
		compat.POST("/cluster/remove", h.RemoveNode)
		compat.GET("/cluster/config", h.GetClusterConfig)
		compat.GET("/cluster/stats", h.GetStatus)
	}

	// ----- 全局状态 (无需认证) -----
	r.GET("/status", h.GetStatus)
	r.GET("/health", h.HealthCheck)

	return r
}

// corsMiddleware 标准 CORS 中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// authMiddleware API 认证中间件
// 支持 X-API-Key header 或 Authorization: Bearer <key>
// 如果配置中未设置 APIKey，则跳过认证
func (h *Handler) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.config.APIKey == "" {
			c.Next()
			return
		}

		key := c.GetHeader("X-API-Key")
		if key == "" {
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if key != h.config.APIKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized: invalid or missing API key",
			})
			return
		}
		c.Next()
	}
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

	// Key 大小校验
	if len(key) > h.config.GetMaxKeySize() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("key size %d exceeds limit %d bytes", len(key), h.config.GetMaxKeySize()),
		})
		return
	}

	var req PutKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Value 大小校验
	if len(req.Value) > h.config.GetMaxValueSize() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("value size %d exceeds limit %d bytes", len(req.Value), h.config.GetMaxValueSize()),
		})
		return
	}

	// 如果不是Leader，返回 Leader 的 HTTP 地址
	if !h.kv.IsLeader() {
		leader := h.kv.GetLeaderHTTPAddr()
		if leader == "" {
			// 无Leader时重试等待
			for i := 0; i < 5; i++ {
				time.Sleep(200 * time.Millisecond)
				leader = h.kv.GetLeaderHTTPAddr()
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
	var req BatchPutRequest

	if err := c.ShouldBindJSON(&req); err != nil {
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

	// 批量大小限制
	if len(req.Items) > h.config.GetMaxBatchSize() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("batch size %d exceeds limit %d", len(req.Items), h.config.GetMaxBatchSize()),
		})
		return
	}

	// Key/Value 大小校验
	maxKeySize := h.config.GetMaxKeySize()
	maxValueSize := h.config.GetMaxValueSize()
	for k, v := range req.Items {
		if len(k) > maxKeySize {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("key '%s' size %d exceeds limit %d bytes", k, len(k), maxKeySize),
			})
			return
		}
		if len(v) > maxValueSize {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("value for key '%s' size %d exceeds limit %d bytes", k, len(v), maxValueSize),
			})
			return
		}
	}

	if !h.kv.IsLeader() {
		c.JSON(http.StatusTemporaryRedirect, gin.H{
			"error":  "not leader",
			"leader": h.kv.GetLeaderHTTPAddr(),
		})
		return
	}

	err := h.retryOnLeaderChange(func() error {
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
			"leader": h.kv.GetLeaderHTTPAddr(),
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

// RunBenchmark 服务端基准测试
func (h *Handler) RunBenchmark(c *gin.Context) {
	var req struct {
		Count       int `json:"count"`
		Concurrency int `json:"concurrency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Count <= 0 {
		req.Count = 5000
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 50
	}
	// 限制最大值防止滥用
	if req.Count > 1000000 {
		req.Count = 1000000
	}
	if req.Concurrency > 500 {
		req.Concurrency = 500
	}

	result, err := h.kv.RunBenchmark(req.Count, req.Concurrency)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetStatus 获取节点状态
func (h *Handler) GetStatus(c *gin.Context) {
	stats := h.kv.GetStats()
	c.JSON(http.StatusOK, stats)
}

// HealthCheck 健康检查（返回真实集群状态）
func (h *Handler) HealthCheck(c *gin.Context) {
	stats := h.kv.GetStats()
	state, _ := stats["state"].(string)
	leader, _ := stats["leader"].(string)

	healthy := state == "Leader" || state == "Follower"
	status := "healthy"
	if !healthy {
		status = "degraded"
	}
	if leader == "" {
		status = "no_leader"
	}

	code := http.StatusOK
	if !healthy {
		code = http.StatusServiceUnavailable
	}

	c.JSON(code, gin.H{
		"status": status,
		"state":  state,
		"leader": leader,
	})
}

// AddNode 添加新节点到集群
func (h *Handler) AddNode(c *gin.Context) {
	var req struct {
		NodeID  string `json:"node_id" binding:"required"`
		Address string `json:"address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 只有 Leader 可以添加节点
	if !h.kv.IsLeader() {
		c.JSON(http.StatusTemporaryRedirect, gin.H{
			"error":  "not leader",
			"leader": h.kv.GetLeaderHTTPAddr(),
		})
		return
	}

	if err := h.kv.AddNode(req.NodeID, req.Address); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "node added successfully",
		"node_id": req.NodeID,
		"address": req.Address,
	})
}

// RemoveNode 从集群移除节点
func (h *Handler) RemoveNode(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 只有 Leader 可以移除节点
	if !h.kv.IsLeader() {
		c.JSON(http.StatusTemporaryRedirect, gin.H{
			"error":  "not leader",
			"leader": h.kv.GetLeaderHTTPAddr(),
		})
		return
	}

	if err := h.kv.RemoveNode(req.NodeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "node removed successfully",
		"node_id": req.NodeID,
	})
}

// GetClusterConfig 获取集群配置
func (h *Handler) GetClusterConfig(c *gin.Context) {
	config, err := h.kv.GetClusterConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, config)
}

// ListKeys 按前缀扫描 Key，支持游标分页
// GET /api/v1/kv?prefix=xxx&limit=100&cursor=xxx
func (h *Handler) ListKeys(c *gin.Context) {
	prefix := c.Query("prefix")
	cursor := c.Query("cursor")
	limit := 100

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	results, nextCursor, err := h.kv.ListKeys(prefix, limit, cursor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":      results,
		"count":      len(results),
		"nextCursor": nextCursor,
	})
}
