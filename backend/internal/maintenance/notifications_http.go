package maintenance

import (
	"campus-resource-platform/backend/internal/maintenance/domain"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
)

func (s *Service) registerNotifications(api *gin.RouterGroup, actor func(*gin.Context) domain.Actor) {
	api.GET("/notifications", func(c *gin.Context) {
		for k, v := range c.Request.URL.Query() {
			if (k != "page" && k != "pageSize" && k != "unread") || len(v) != 1 {
				respond(c, 0, nil, fault("INVALID_INPUT", 400))
				return
			}
		}
		p, e := strconv.Atoi(c.DefaultQuery("page", "1"))
		if e != nil {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		size, e := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
		if e != nil {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		unread := c.DefaultQuery("unread", "false")
		if unread != "true" && unread != "false" {
			respond(c, 0, nil, fault("INVALID_INPUT", 400))
			return
		}
		v, e := s.Notifications(c.Request.Context(), actor(c), p, size, unread == "true")
		respond(c, 200, v, e)
	})
	api.POST("/notifications/:eventId/read", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var q struct {
			Kind string `json:"kind"`
		}
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
		e := s.ReadNotification(c.Request.Context(), actor(c), c.Param("eventId"), q.Kind)
		respond(c, 200, gin.H{"read": true}, e)
	})
}
