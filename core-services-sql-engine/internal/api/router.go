// Package api wires up the Echo HTTP server for the digestion engine.
package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/manju-backend/core-services-sql-engine/internal/api/handlers"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion"
)

// New creates a configured Echo instance with all routes registered.
func New(engine *digestion.Engine, scheduler *digestion.Scheduler) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	h := handlers.New(engine, scheduler)

	// Health
	e.GET("/health", h.Health)

	// Sources
	e.GET("/sources", h.ListSources)
	e.POST("/sources", h.CreateSource)
	e.GET("/sources/:sourceId", h.GetSource)
	e.DELETE("/sources/:sourceId", h.DeleteSource)

	// Discovery
	e.POST("/sources/:sourceId/discover", h.Discover)

	// Tables & columns
	e.GET("/sources/:sourceId/tables", h.ListTables)
	e.GET("/sources/:sourceId/tables/:tableId", h.GetTable)
	e.GET("/sources/:sourceId/tables/:tableId/columns", h.ListColumns)

	// Sync
	e.POST("/sources/:sourceId/sync", h.SyncSource)
	e.POST("/sources/:sourceId/tables/:tableId/sync", h.SyncTable)

	// Sync jobs
	e.GET("/sources/:sourceId/jobs", h.ListSyncJobs)
	e.GET("/jobs/:jobId", h.GetSyncJob)

	// Data & changes
	e.GET("/sources/:sourceId/tables/:tableId/data", h.QueryData)
	e.GET("/sources/:sourceId/tables/:tableId/changes", h.ListChanges)
	e.GET("/sources/:sourceId/tables/:tableId/snapshots", h.ListSnapshots)

	return e
}
