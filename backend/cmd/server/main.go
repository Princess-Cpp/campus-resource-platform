package main

import (
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/internal/m1"
	"campus-resource-platform/backend/internal/m2"
	"campus-resource-platform/backend/migrations"
	"context"
	"database/sql"
	"embed"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

//go:embed webdist/*
var web embed.FS

func main() {
	if os.Getenv("M1_DEV_MODE") != "true" {
		log.Fatal("M1_DEV_MODE=true is required until M2 identity/catalog is integrated")
	}
	secret := os.Getenv("M1_CODE_SECRET")
	if len(secret) < 32 {
		log.Fatal("M1_CODE_SECRET must contain 32 bytes")
	}
	f, e := dev.Load(os.Getenv("M1_DEV_FIXTURE"))
	if e != nil {
		log.Fatal(e)
	}
	db, e := sql.Open("mysql", os.Getenv("MYSQL_DSN"))
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	ctx := context.Background()
	if e = db.PingContext(ctx); e != nil {
		log.Fatal(e)
	}
	if e = migrations.Apply(ctx, db); e != nil {
		log.Fatal(e)
	}
	s := m1.New(db, f, []byte(secret))
	r := s.Router(f.Auth)
	if os.Getenv("M2_ENABLED") == "true" {
		directory, err := dev.LoadM2(os.Getenv("M1_DEV_FIXTURE"))
		if err != nil {
			log.Fatal(err)
		}
		base, token := os.Getenv("M2_RESOURCE_URL"), os.Getenv("M2_SERVICE_TOKEN")
		if base == "" || token == "" {
			log.Fatal("M2_RESOURCE_URL and M2_SERVICE_TOKEN are required")
		}
		classroom := m2.New(db, directory, &m2.HTTPResources{BaseURL: base, Token: token})
		classroom.Policy = directory.Policy
		classroom.Register(r)
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if err := classroom.Tick(ctx); err != nil {
					log.Printf("M2 task error: %v", err)
				}
			}
		}()
	}
	webRoot, e := fs.Sub(web, "webdist")
	if e != nil {
		log.Fatal(e)
	}
	static := http.FileServer(http.FS(webRoot))
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			c.Status(404)
			return
		}
		// 前端使用 History 路由：无扩展名且不存在的路径回退到 index.html，缺失的静态资源仍返回 404。
		if _, e := fs.Stat(webRoot, strings.TrimPrefix(path.Clean(p), "/")); e != nil && path.Ext(p) == "" {
			c.Request.URL.Path = "/"
		}
		static.ServeHTTP(c.Writer, c.Request)
	})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			if e := s.Tick(ctx); e != nil {
				log.Printf("M1 task retry: %v", e)
			}
		}
	}()
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("M1 development server on %s", addr)
	log.Fatal((&http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
