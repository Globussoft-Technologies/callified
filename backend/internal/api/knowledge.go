package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// GET /api/knowledge
func (s *Server) listKnowledge(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	files, err := s.db.GetKnowledgeFiles(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(files))
}

// knowledgeStoragePath returns the absolute path where a knowledge file
// is (or would be) stored on disk. We embed the file id in the filename
// so two uploads with the same client-side name don't collide.
func (s *Server) knowledgeStoragePath(orgID, fileID int64, filename string) string {
	return filepath.Join(
		s.cfg.KnowledgeDir,
		strconv.FormatInt(orgID, 10),
		fmt.Sprintf("%d_%s", fileID, filename),
	)
}

// POST /api/knowledge  — multipart upload
func (s *Server) uploadKnowledge(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	// Sanitize
	if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".pdf" && ext != ".txt" && ext != ".docx" {
		writeError(w, http.StatusBadRequest, "only PDF, TXT, and DOCX files supported")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, 20<<20)) // 20 MB limit
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read error")
		return
	}

	// Log to DB (need the row id for the storage path)
	fileID, err := s.db.LogKnowledgeFile(ac.OrgID, filename, ext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Persist the original bytes to disk so they can be re-served via
	// /api/knowledge/{id}/download. Failure to save is non-fatal — RAG
	// embedding still works without the file on disk; the user just
	// can't preview/download it later.
	storagePath := s.knowledgeStoragePath(ac.OrgID, fileID, filename)
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
		s.logger.Warn("uploadKnowledge: mkdir", zap.Error(err))
	} else if err := os.WriteFile(storagePath, data, 0o644); err != nil {
		s.logger.Warn("uploadKnowledge: write file", zap.Error(err), zap.String("path", storagePath))
	}

	// Send to RAG service asynchronously. Use a detached context with a
	// generous timeout — r.Context() gets canceled the moment the
	// handler returns its response, which would abort the in-flight
	// IngestPDF POST before it completes (manifests as
	// "context canceled" → status='failed' → 0 FAISS chunks).
	if s.ragClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := s.ragClient.IngestPDF(ctx, ac.OrgID, filename, data); err != nil {
				s.logger.Warn("uploadKnowledge: ingest failed",
					zap.String("file", filename), zap.Error(err))
				_ = s.db.UpdateKnowledgeFileStatus(fileID, "failed")
			} else {
				_ = s.db.UpdateKnowledgeFileStatus(fileID, "indexed")
			}
		}()
	}

	// Response shape mirrors Python routes.py:1283 — the frontend checks
	// `data.status === 'success'`; anything else renders as an error.
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":   "success",
		"message":  "File is being processed automatically in the background.",
		"file_id":  fileID,
		"filename": filename,
	})
}

// GET /api/knowledge/{id}/download
//
// Serves the original uploaded file. Auth-gated; accepts the JWT via
// Authorization header (default) or ?token=... (so a plain anchor in a
// new tab works without needing fetch+blob plumbing on the client).
func (s *Server) downloadKnowledge(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	kf, err := s.db.GetKnowledgeFileByID(id, ac.OrgID)
	if err != nil || kf == nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	storagePath := s.knowledgeStoragePath(ac.OrgID, kf.ID, kf.Filename)
	if _, err := os.Stat(storagePath); err != nil {
		// Older uploads (pre-storage) won't have a file on disk. Be
		// explicit so the user understands re-upload is needed.
		writeError(w, http.StatusGone, "original file not stored — please re-upload")
		return
	}
	// Inline the PDF (browsers preview it); leave Content-Disposition off so
	// browsers can choose to render rather than force a download.
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=%q", kf.Filename))
	http.ServeFile(w, r, storagePath)
}

// DELETE /api/knowledge/{id}
func (s *Server) deleteKnowledge(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	kf, err := s.db.GetKnowledgeFileByID(id, ac.OrgID)
	if err != nil || kf == nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// Remove from RAG index. Detached context for the same reason as
	// uploadKnowledge — r.Context() is canceled when this handler returns.
	if s.ragClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = s.ragClient.RemoveFile(ctx, ac.OrgID, kf.Filename)
		}()
	}

	// Best-effort delete from disk; not having a file there isn't an error.
	storagePath := s.knowledgeStoragePath(ac.OrgID, kf.ID, kf.Filename)
	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("deleteKnowledge: remove file", zap.Error(err))
	}

	if err := s.db.DeleteKnowledgeFile(id, ac.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
