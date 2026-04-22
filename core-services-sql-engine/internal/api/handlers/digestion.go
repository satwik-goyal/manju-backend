// Package handlers contains the Echo HTTP handlers for the digestion engine API.
package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion"
)

// Handler wraps the digestion Engine and Scheduler for HTTP delivery.
type Handler struct {
	engine    *digestion.Engine
	scheduler *digestion.Scheduler
}

// New creates a Handler.
func New(engine *digestion.Engine, scheduler *digestion.Scheduler) *Handler {
	return &Handler{engine: engine, scheduler: scheduler}
}

// --- helpers ----------------------------------------------------------------

func ok(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, map[string]any{"data": data})
}

func created(c echo.Context, data any) error {
	return c.JSON(http.StatusCreated, map[string]any{"data": data})
}

func errResp(c echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]any{"error": msg})
}

func parseID(c echo.Context, param string) (uuid.UUID, error) {
	return uuid.Parse(c.Param(param))
}

// --- Sources ----------------------------------------------------------------

func (h *Handler) ListSources(c echo.Context) error {
	sources, err := h.engine.ListSources(c.Request().Context())
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, sources)
}

func (h *Handler) GetSource(c echo.Context) error {
	id, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	src, err := h.engine.GetSource(c.Request().Context(), id)
	if err != nil {
		return errResp(c, http.StatusNotFound, "source not found")
	}
	return ok(c, src)
}

func (h *Handler) CreateSource(c echo.Context) error {
	var req digestion.RegisterSourceRequest
	if err := c.Bind(&req); err != nil {
		return errResp(c, http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" || req.DBType == "" || req.Host == "" || req.DatabaseName == "" {
		return errResp(c, http.StatusBadRequest, "name, db_type, host, and database_name are required")
	}
	if req.Port == 0 {
		req.Port = 5432
	}

	src, err := h.engine.RegisterSource(c.Request().Context(), req)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}

	// Optionally schedule it right away.
	if src.SyncEnabled {
		_ = h.scheduler.ScheduleSource(src.ID, src.SyncIntervalSeconds)
	}

	return created(c, src)
}

func (h *Handler) DeleteSource(c echo.Context) error {
	id, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	h.scheduler.UnscheduleSource(id)
	if err := h.engine.DeleteSource(c.Request().Context(), id); err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Discovery --------------------------------------------------------------

func (h *Handler) Discover(c echo.Context) error {
	id, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	if err := h.engine.Discover(c.Request().Context(), id); err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	tables, err := h.engine.GetTables(c.Request().Context(), id)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, map[string]any{"tables": tables})
}

// --- Tables -----------------------------------------------------------------

func (h *Handler) ListTables(c echo.Context) error {
	id, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	tables, err := h.engine.GetTables(c.Request().Context(), id)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, tables)
}

func (h *Handler) GetTable(c echo.Context) error {
	id, err := parseID(c, "tableId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid table ID")
	}
	table, err := h.engine.GetTable(c.Request().Context(), id)
	if err != nil {
		return errResp(c, http.StatusNotFound, "table not found")
	}
	return ok(c, table)
}

func (h *Handler) ListColumns(c echo.Context) error {
	id, err := parseID(c, "tableId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid table ID")
	}
	cols, err := h.engine.GetColumns(c.Request().Context(), id)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, cols)
}

// --- Sync -------------------------------------------------------------------

func (h *Handler) SyncSource(c echo.Context) error {
	id, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	// Run sync in a goroutine and return 202 immediately.
	go func() {
		if err := h.engine.SyncSource(c.Request().Context(), id); err != nil {
			// already logged inside SyncSource
			_ = err
		}
	}()
	return c.JSON(http.StatusAccepted, map[string]any{"message": "sync started"})
}

func (h *Handler) SyncTable(c echo.Context) error {
	sourceID, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	tableID, err := parseID(c, "tableId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid table ID")
	}
	job, err := h.engine.SyncTable(c.Request().Context(), sourceID, tableID)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, job)
}

// --- Data & Changes ---------------------------------------------------------

func (h *Handler) QueryData(c echo.Context) error {
	id, err := parseID(c, "tableId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid table ID")
	}
	limit := bindInt(c, "limit", 100)
	offset := bindInt(c, "offset", 0)

	rows, err := h.engine.GetData(c.Request().Context(), id, limit, offset)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, rows)
}

func (h *Handler) ListChanges(c echo.Context) error {
	id, err := parseID(c, "tableId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid table ID")
	}
	limit := bindInt(c, "limit", 100)
	changes, err := h.engine.GetChanges(c.Request().Context(), id, limit)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, changes)
}

func (h *Handler) ListSnapshots(c echo.Context) error {
	id, err := parseID(c, "tableId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid table ID")
	}
	limit := bindInt(c, "limit", 20)
	snaps, err := h.engine.GetSnapshots(c.Request().Context(), id, limit)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, snaps)
}

// --- Sync Jobs --------------------------------------------------------------

func (h *Handler) ListSyncJobs(c echo.Context) error {
	id, err := parseID(c, "sourceId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid source ID")
	}
	limit := bindInt(c, "limit", 20)
	jobs, err := h.engine.GetSyncJobs(c.Request().Context(), id, limit)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, jobs)
}

func (h *Handler) GetSyncJob(c echo.Context) error {
	id, err := parseID(c, "jobId")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid job ID")
	}
	job, err := h.engine.GetSyncJob(c.Request().Context(), id)
	if err != nil {
		return errResp(c, http.StatusNotFound, "job not found")
	}
	return ok(c, job)
}

// --- Health -----------------------------------------------------------------

func (h *Handler) Health(c echo.Context) error {
	return ok(c, map[string]string{"status": "ok"})
}

// bindInt reads a query param as an int with a default.
func bindInt(c echo.Context, param string, def int) int {
	v := c.QueryParam(param)
	if v == "" {
		return def
	}
	n := def
	_ = echo.QueryParamsBinder(c).Int(param, &n).BindError()
	return n
}
