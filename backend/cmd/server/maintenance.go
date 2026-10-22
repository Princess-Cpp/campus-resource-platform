package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
	maintenancedev "campus-resource-platform/backend/internal/maintenance/dev"
	"github.com/gin-gonic/gin"
)

func registerMaintenance(router *gin.Engine, db *sql.DB, store *identity.Store) error {
	if os.Getenv("M3_ENABLED") != "true" {
		return nil
	}
	// The current catalog/transaction-event providers are explicit development
	// adapters. Fail closed instead of quietly using them as production services.
	if os.Getenv("M3_DEV_MODE") != "true" || os.Getenv("M1_DEV_MODE") != "true" {
		return fmt.Errorf("M3 requires explicit M3_DEV_MODE=true until shared catalog/events are connected")
	}
	catalog, e := maintenancedev.LoadCatalog(os.Getenv("M3_DEV_FIXTURE"))
	if e != nil {
		return e
	}
	staff, e := maintenancedev.LoadStaff(os.Getenv("M3_DEV_STAFF_FIXTURE"))
	if e != nil {
		return e
	}
	directory := &maintenancedev.Directory{Store: store, StudentReporting: os.Getenv("M3_DEV_STUDENT_REPORTING") == "true", Staff: staff}
	service := maintenance.New(db, catalog, maintenancedev.Events{})
	service.Development = true
	service.People = directory
	service.Supervisors = directory
	endpoint, token := os.Getenv("M3_RESOURCE_URL"), os.Getenv("M3_SERVICE_TOKEN")
	if endpoint != "" || token != "" {
		resource, err := maintenance.NewHTTPResources(endpoint, token)
		if err != nil {
			return err
		}
		service.Resources = resource
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			if err := service.Sweep(ctx); err != nil {
				log.Print("M3 background processing did not commit; persisted work will be retried")
			}
			cancel()
		}
	}()
	service.Register(router, directory.Authenticate)
	return nil
}
