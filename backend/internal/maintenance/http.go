package maintenance

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"

	"campus-resource-platform/backend/internal/maintenance/domain"
	"github.com/gin-gonic/gin"
)

func respond(c *gin.Context, status int, value any, e error) {
	if e != nil {
		status = 500
		code := "INTERNAL_ERROR"
		var f *Fault
		if errors.As(e, &f) {
			status, code = f.Status, f.Code
		}
		c.JSON(status, gin.H{"error": gin.H{"code": code}, "correlationId": c.GetString("trace")})
		return
	}
	c.JSON(status, gin.H{"data": value, "correlationId": c.GetString("trace")})
}
func (s *Service) Register(r *gin.Engine, authenticate Authenticate) {
	api := r.Group("/api/v1/m3")
	api.Use(func(c *gin.Context) {
		trace := c.GetHeader("X-Correlation-Id")
		if len(trace) > 64 || !keyPattern.MatchString(trace) {
			trace = ID()
		}
		c.Set("trace", trace)
		c.Header("X-Correlation-Id", trace)
		a, e := authenticate(c.Request.Context(), c.Request)
		if e != nil || !a.Enabled || a.ID == "" {
			respond(c, 0, nil, fault("UNAUTHENTICATED", 401))
			c.Abort()
			return
		}
		c.Set("maintenanceActor", a)
	})
	actor := func(c *gin.Context) domain.Actor { return c.MustGet("maintenanceActor").(domain.Actor) }
	api.GET("/session", func(c *gin.Context) {
		a := actor(c)
		respond(c, 200, gin.H{"actor": gin.H{"id": a.ID, "permissions": a.Permissions}, "development": s.Development}, nil)
	})
	api.GET("/rooms", func(c *gin.Context) { v, e := s.Rooms(c.Request.Context(), actor(c)); respond(c, 200, v, e) })
	api.POST("/workorders", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var q ReportInput
		d := json.NewDecoder(c.Request.Body)
		d.DisallowUnknownFields()
		if e := d.Decode(&q); e != nil {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		if e := d.Decode(new(any)); e != io.EOF {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Report(c.Request.Context(), actor(c), q, c.GetHeader("Idempotency-Key"), c.GetString("trace"))
		respond(c, 201, v, e)
	})
	api.GET("/workorders", func(c *gin.Context) {
		allowed := map[string]bool{"view": true, "page": true, "pageSize": true, "state": true, "category": true, "from": true, "to": true}
		for name, values := range c.Request.URL.Query() {
			if !allowed[name] || len(values) != 1 {
				respond(c, 0, nil, fault("INVALID_INPUT", 400))
				return
			}
		}
		if view := c.Query("view"); view != "" && !slices.Contains([]string{"mine", "dispatch", "assigned", "acceptance"}, view) {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
		if e != nil {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		size, e := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
		if e != nil {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.List(c.Request.Context(), actor(c), Filters{View: c.Query("view"), Page: page, PageSize: size, State: c.Query("state"), Category: c.Query("category"), From: c.Query("from"), To: c.Query("to")})
		respond(c, 200, v, e)
	})
	api.GET("/workorders/:id/assignees", func(c *gin.Context) {
		v, e := s.Assignees(c.Request.Context(), actor(c), c.Param("id"))
		respond(c, 200, v, e)
	})
	api.POST("/workorders/:id/actions", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var q ActionInput
		d := json.NewDecoder(c.Request.Body)
		d.DisallowUnknownFields()
		if e := d.Decode(&q); e != nil {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		if e := d.Decode(new(any)); e != io.EOF {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Act(c.Request.Context(), actor(c), c.Param("id"), q, c.GetHeader("Idempotency-Key"), c.GetString("trace"))
		status := 200
		if e == nil && v.SyncState != domain.Idle {
			status = 202
		}
		respond(c, status, v, e)
	})

	api.GET("/workorders/:id", func(c *gin.Context) {
		v, e := s.Get(c.Request.Context(), actor(c), c.Param("id"))
		respond(c, 200, v, e)
	})
}
