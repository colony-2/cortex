package cortex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Registry responses deliberately retain JobDB's canonical shape and cursors.
func (s *Server) handleListCells(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := jobdb.ListCellsRequest{TenantID: s.tenantID(r), Name: q.Get("name"), State: q.Get("state"), PageToken: q.Get("pageToken")}
	if q.Has("pageSize") {
		size, err := strconv.Atoi(q.Get("pageSize"))
		if err != nil || size < 1 || size > 1000 {
			writeError(w, jobdb.ErrInvalidCell)
			return
		}
		req.PageSize = size
	}
	if q.Has("isRoot") {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("isRoot is no longer supported; cells have no special root role"))
		return
	}
	cells, err := s.cells.ListCells(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	if cells.Cells == nil {
		cells.Cells = []jobdb.Cell{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, cells)
}

func (s *Server) handleGetCell(w http.ResponseWriter, r *http.Request) {
	cell, err := s.cells.GetCell(r.Context(), jobdb.CellKey{TenantID: s.tenantID(r), CellID: mux.Vars(r)["cellId"]})
	if err != nil {
		writeError(w, err)
		return
	}
	writeCell(w, http.StatusOK, cell)
}

func (s *Server) handleCreateCell(w http.ResponseWriter, r *http.Request) {
	var req jobdb.CreateCellRequest
	if err := decodeRequest(w, r, &req); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	repo, err := jobdb.NormalizeCellRepo(req.Repo)
	if err != nil {
		writeError(w, err)
		return
	}
	req.TenantID, req.Repo = s.tenantID(r), repo
	cell, err := s.cells.CreateCell(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeCell(w, http.StatusCreated, cell)
}

func writeCell(w http.ResponseWriter, status int, cell jobdb.Cell) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", fmt.Sprintf("%q", cell.Revision))
	writeJSON(w, status, cell)
}

func decodeRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON object")
	}
	return nil
}
