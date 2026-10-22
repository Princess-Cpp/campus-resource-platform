package classroom

import (
	"campus-resource-platform/backend/internal/identity"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

func reply(c *gin.Context, v any, e error) {
	if e != nil {
		status, code := 500, "INTERNAL_ERROR"
		var f *Fault
		if errors.As(e, &f) {
			status, code = f.Status, f.Code
		}
		c.JSON(status, gin.H{"error": gin.H{"code": code}, "correlationId": c.GetString("trace")})
		return
	}
	c.JSON(200, gin.H{"data": v, "correlationId": c.GetString("trace")})
}
func (s *Service) Register(r *gin.Engine) {
	api := r.Group("/api/v1/m2")
	api.Use(func(c *gin.Context) {
		token, ok := identity.Credential(c.Request)
		if !ok {
			reply(c, nil, fault("UNAUTHENTICATED", 401))
			c.Abort()
			return
		}
		a, e := s.Directory.Authenticate(c.Request.Context(), token)
		if e != nil {
			reply(c, nil, fault("UNAUTHENTICATED", 401))
			c.Abort()
			return
		}
		c.Set("classroomActor", a)
		c.Set("m2Token", token)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	})
	actor := func(c *gin.Context) Actor { return c.MustGet("classroomActor").(Actor) }
	api.GET("/session", func(c *gin.Context) { reply(c, gin.H{"actor": actor(c), "policy": s.Policy, "development": true}, nil) })
	api.GET("/rooms", func(c *gin.Context) {
		start, e := time.Parse(time.RFC3339, c.Query("startAt"))
		if e != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		end, e := time.Parse(time.RFC3339, c.Query("endAt"))
		if e != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		n, e := strconv.Atoi(c.DefaultQuery("capacity", "1"))
		if e != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Search(c.Request.Context(), actor(c), c.GetString("m2Token"), start, end, c.Query("area"), n)
		reply(c, v, e)
	})
	api.POST("/applications", func(c *gin.Context) {
		var q CreateRequest
		if c.ShouldBindJSON(&q) != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Create(c.Request.Context(), actor(c), c.GetString("m2Token"), q, c.GetHeader("Idempotency-Key"), c.GetString("trace"))
		reply(c, v, e)
	})
	filters := func(c *gin.Context) Filters {
		p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		n, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
		return Filters{Status: c.Query("status"), From: c.Query("from"), To: c.Query("to"), RoomID: c.Query("roomId"), Page: p, PageSize: n}
	}
	api.GET("/applications", func(c *gin.Context) { v, e := s.List(c.Request.Context(), actor(c), filters(c)); reply(c, v, e) })
	api.GET("/approvals", func(c *gin.Context) {
		f := filters(c)
		f.Inbox = true
		v, e := s.List(c.Request.Context(), actor(c), f)
		reply(c, v, e)
	})
	api.GET("/applications/:id", func(c *gin.Context) { v, e := s.Get(c.Request.Context(), actor(c), c.Param("id")); reply(c, v, e) })
	api.DELETE("/applications/:id", func(c *gin.Context) {
		id := c.Param("id")
		e := s.Hide(c.Request.Context(), actor(c), id, c.GetString("trace"))
		reply(c, gin.H{"id": id}, e)
	})
	api.POST("/applications/:id/actions", func(c *gin.Context) {
		var q ActionRequest
		if c.ShouldBindJSON(&q) != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Action(c.Request.Context(), actor(c), c.Param("id"), q, c.GetHeader("Idempotency-Key"), c.GetString("trace"))
		reply(c, v, e)
	})
	api.POST("/applications/:id/retry", func(c *gin.Context) {
		var q struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		if c.ShouldBindJSON(&q) != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Retry(c.Request.Context(), actor(c), c.Param("id"), q.ExpectedVersion)
		reply(c, v, e)
	})
	api.GET("/stats", func(c *gin.Context) { v, e := s.Stats(c.Request.Context(), actor(c), filters(c)); reply(c, v, e) })
}
