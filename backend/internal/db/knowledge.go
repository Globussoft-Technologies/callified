package db

import (
	"database/sql"
	"errors"
)

// KnowledgeFile mirrors the knowledge_base table (Python's canonical name —
// earlier queries hit `knowledge_files` which doesn't exist in the schema,
// so every GET /api/knowledge silently returned 500 and the RAG panel kept
// showing "No documents in the vector database yet" no matter what).
//
// `ChunkCount` is what the frontend renders as "✅ Active (N FAISS Chunks)"
// on each row — add the column to the struct so the JSON payload carries it.
type KnowledgeFile struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	Filename   string `json:"filename"`
	ChunkCount int64  `json:"chunk_count"`
	Status     string `json:"status"` // Processing, Active, failed
	CreatedAt  string `json:"created_at"`
}

// LogKnowledgeFile inserts a knowledge file record. Returns new ID.
// fileType is accepted for API compatibility with the upload handler but
// the DB schema doesn't store it — Python doesn't either.
func (d *DB) LogKnowledgeFile(orgID int64, filename, _fileType string) (int64, error) {
	res, err := d.pool.Exec(`
		INSERT INTO knowledge_base (org_id, filename, chunk_count, status)
		VALUES (?,?,0,'Processing')`, orgID, filename)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateKnowledgeFileStatus updates the indexing status of a file. When the
// RAG sidecar succeeds we also flip status to "Active" to match the string
// the frontend compares against.
func (d *DB) UpdateKnowledgeFileStatus(id int64, status string) error {
	if status == "indexed" {
		status = "Active"
	}
	_, err := d.pool.Exec(
		`UPDATE knowledge_base SET status=? WHERE id=?`, status, id)
	return err
}

// UpdateKnowledgeFileChunks sets the chunk_count after ingestion.
func (d *DB) UpdateKnowledgeFileChunks(id int64, chunkCount int64) error {
	_, err := d.pool.Exec(
		`UPDATE knowledge_base SET chunk_count=?, status='Active' WHERE id=?`,
		chunkCount, id)
	return err
}

// GetKnowledgeFiles returns all knowledge files for an org.
func (d *DB) GetKnowledgeFiles(orgID int64) ([]KnowledgeFile, error) {
	rows, err := d.pool.Query(`
		SELECT id, org_id, COALESCE(filename,''),
		COALESCE(chunk_count,0), COALESCE(status,'Processing'),
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM knowledge_base WHERE org_id=? ORDER BY id DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []KnowledgeFile
	for rows.Next() {
		var kf KnowledgeFile
		if err := rows.Scan(&kf.ID, &kf.OrgID, &kf.Filename,
			&kf.ChunkCount, &kf.Status, &kf.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, kf)
	}
	return list, rows.Err()
}

// GetKnowledgeFileByID returns a single knowledge file record.
func (d *DB) GetKnowledgeFileByID(id, orgID int64) (*KnowledgeFile, error) {
	row := d.pool.QueryRow(`
		SELECT id, org_id, COALESCE(filename,''),
		COALESCE(chunk_count,0), COALESCE(status,'Processing'),
		DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s')
		FROM knowledge_base WHERE id=? AND org_id=?`, id, orgID)
	var kf KnowledgeFile
	err := row.Scan(&kf.ID, &kf.OrgID, &kf.Filename,
		&kf.ChunkCount, &kf.Status, &kf.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &kf, err
}

// DeleteKnowledgeFile removes a knowledge file record.
func (d *DB) DeleteKnowledgeFile(id, orgID int64) error {
	_, err := d.pool.Exec(
		`DELETE FROM knowledge_base WHERE id=? AND org_id=?`, id, orgID)
	return err
}
