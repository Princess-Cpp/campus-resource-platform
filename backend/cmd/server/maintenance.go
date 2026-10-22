package main

import (
	"database/sql"
	"fmt"
	"os"

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
	service := maintenance.New(db, catalog, maintenancedev.Events{})
	service.Development = true
	service.Register(router, maintenancedev.Authenticator(store, os.Getenv("M3_DEV_STUDENT_REPORTING") == "true"))
	return nil
}
