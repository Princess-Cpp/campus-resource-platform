package booking

import (
	"campus-resource-platform/backend/internal/identity"
	"context"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Authenticate func(context.Context, string) (Actor, error)

func (s *Service) Router(auth Authenticate, before ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		t := c.GetHeader("X-Correlation-Id")
		if len(t) == 0 || len(t) > 64 {
			t = ID()
		}
		c.Set("trace", t)
		c.Header("X-Correlation-Id", t)
	})
	r.Use(before...)
	r.GET("/health/ready", func(c *gin.Context) {
		if e := s.DB.PingContext(c.Request.Context()); e != nil {
			c.Status(503)
			return
		}
		c.JSON(200, gin.H{"ready": true})
	})
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		t, ok := identity.Credential(c.Request)
		if !ok {
			s.reply(c, nil, bad("UNAUTHENTICATED", 401))
			c.Abort()
			return
		}
		a, e := auth(c.Request.Context(), t)
		if e != nil {
			s.reply(c, nil, bad("UNAUTHENTICATED", 401))
			c.Abort()
			return
		}
		if _, cookieErr := c.Request.Cookie(identity.CookieName); cookieErr == nil && a.Service != "" {
			s.reply(c, nil, bad("UNAUTHENTICATED", 401))
			c.Abort()
			return
		}
		c.Set("actor", a)
	})
	get := func(c *gin.Context) Actor { return c.MustGet("actor").(Actor) }
	user := func(c *gin.Context) bool {
		if get(c).Service != "" {
			s.reply(c, nil, bad("FORBIDDEN", 403))
			return false
		}
		return true
	}
	api.GET("/resources/availability", func(c *gin.Context) {
		a, e := time.Parse(time.RFC3339, c.Query("startAt"))
		if e != nil {
			s.reply(c, nil, bad("INVALID_INPUT", 400))
			return
		}
		b, e := time.Parse(time.RFC3339, c.Query("endAt"))
		if e != nil {
			s.reply(c, nil, bad("INVALID_INPUT", 400))
			return
		}
		v, e := s.Search(c.Request.Context(), get(c), a, b, c.Query("area"), c.Query("outlet") == "true")
		s.reply(c, v, e)
	})
	api.POST("/m1/bookings", func(c *gin.Context) {
		if !user(c) {
			return
		}
		if !get(c).Can("booking.create") {
			s.reply(c, nil, bad("FORBIDDEN", 403))
			return
		}
		var q BookRequest
		if c.ShouldBindJSON(&q) != nil {
			s.reply(c, nil, bad("INVALID_INPUT", 400))
			return
		}
		v, e := s.Book(c.Request.Context(), get(c), q, c.GetHeader("Idempotency-Key"), c.GetString("trace"))
		s.reply(c, v, e)
	})
	api.GET("/m1/bookings", func(c *gin.Context) {
		if !user(c) {
			return
		}
		p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		n, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
		if p < 1 || n < 1 || n > 100 {
			s.reply(c, nil, bad("INVALID_INPUT", 400))
			return
		}
		v, e := s.History(c.Request.Context(), get(c), p, n)
		s.reply(c, v, e)
	})
	api.POST("/m1/bookings/:id/check-in", func(c *gin.Context) {
		if !user(c) {
			return
		}
		var q struct {
			Code string `json:"code"`
		}
		if c.ShouldBindJSON(&q) != nil || q.Code == "" {
			s.reply(c, nil, bad("INVALID_INPUT", 400))
			return
		}
		v, e := s.Change(c.Request.Context(), get(c), c.Param("id"), "CHECK_IN", strings.ToUpper(q.Code), c.GetString("trace"))
		s.reply(c, v, e)
	})
	api.POST("/m1/bookings/:id/cancel", func(c *gin.Context) {
		if !user(c) {
			return
		}
		v, e := s.Change(c.Request.Context(), get(c), c.Param("id"), "CANCEL", "", c.GetString("trace"))
		s.reply(c, v, e)
	})
	api.DELETE("/m1/bookings/:id", func(c *gin.Context) {
		if !user(c) {
			return
		}
		id := c.Param("id")
		e := s.Hide(c.Request.Context(), get(c), id, c.GetString("trace"))
		s.reply(c, gin.H{"id": id}, e)
	})
	api.GET("/m1/notifications", func(c *gin.Context) {
		if !user(c) {
			return
		}
		v, e := s.Notifications(c.Request.Context(), get(c))
		s.reply(c, v, e)
	})
	internal := api.Group("/internal/resources")
	internal.Use(func(c *gin.Context) {
		if get(c).Service != "M2" && get(c).Service != "M3" {
			s.reply(c, nil, bad("FORBIDDEN", 403))
			c.Abort()
		}
	})
	internal.POST("/operations", func(c *gin.Context) {
		var q Operation
		if c.ShouldBindJSON(&q) != nil {
			s.reply(c, nil, bad("INVALID_INPUT", 400))
			return
		}
		v, e := s.Operate(c.Request.Context(), get(c), q, c.GetHeader("Idempotency-Key"), c.GetString("trace"))
		if e == nil && v.ErrorCode != "" {
			c.JSON(409, gin.H{"data": v, "error": gin.H{"code": v.ErrorCode}, "correlationId": c.GetString("trace")})
			return
		}
		s.reply(c, v, e)
	})
	internal.GET("/operations/:id", func(c *gin.Context) { v, e := s.Receipt(c.Request.Context(), get(c), c.Param("id")); s.reply(c, v, e) })
	internal.GET("/sources", func(c *gin.Context) { v, e := s.Sources(c.Request.Context(), get(c)); s.reply(c, v, e) })
	return r
}
func (s *Service) reply(c *gin.Context, v any, e error) {
	if e != nil {
		status, code := 500, "INTERNAL_ERROR"
		var f *Fault
		var m *mysql.MySQLError
		if errors.As(e, &f) {
			status, code = f.Status, f.Code
		} else if errors.As(e, &m) && m.Number == 1062 {
			status, code = 409, "IDEMPOTENCY_CONFLICT"
		}
		c.JSON(status, gin.H{"error": gin.H{"code": code}, "correlationId": c.GetString("trace")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v, "correlationId": c.GetString("trace")})
}

var _ = sql.ErrNoRows
