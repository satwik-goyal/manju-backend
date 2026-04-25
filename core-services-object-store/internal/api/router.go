// Package api wires up the Echo HTTP server for the Object Store Service.
package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/manju-backend/core-services-object-store/internal/api/handlers"
	"github.com/manju-backend/core-services-object-store/pkg/filestore"
)

// New creates a configured Echo instance with all Object Store routes registered.
func New(store *filestore.Store) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	h := handlers.New(store)

	e.GET("/health", h.Health)

	v1 := e.Group("/v1")

	// File operations — register /duplicate before /:id so Echo's static
	// route wins over the parameterised one.
	v1.POST("/files", h.UploadFile)
	v1.GET("/files", h.ListFiles)
	v1.GET("/files/duplicate", h.FindDuplicate)
	v1.GET("/files/:id", h.GetFile)
	v1.DELETE("/files/:id", h.DeleteFile)
	v1.GET("/files/:id/download", h.DownloadFile)
	v1.GET("/files/:id/presign", h.PresignFile)
	v1.PATCH("/files/:id/status", h.UpdateFileStatus)

	// Folder/prefix browsing
	v1.GET("/browse", h.Browse)

	return e
}
