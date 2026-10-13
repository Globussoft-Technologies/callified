package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const productResearchSchema = `CREATE TABLE IF NOT EXISTS product_research_jobs (
 id VARCHAR(64) PRIMARY KEY, org_id BIGINT NOT NULL, product_id BIGINT NOT NULL,
 website_url TEXT NOT NULL, base_knowledge LONGTEXT NOT NULL,
 status VARCHAR(20) NOT NULL, extracted LONGTEXT NOT NULL, summary LONGTEXT NOT NULL,
 error_message TEXT NOT NULL, created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
 INDEX idx_product_research (org_id, product_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

type ProductResearchJob struct {
	ID            string    `json:"id"`
	OrgID         int64     `json:"-"`
	ProductID     int64     `json:"product_id"`
	URL           string    `json:"website_url"`
	BaseKnowledge string    `json:"-"`
	Status        string    `json:"status"`
	Extracted     string    `json:"-"`
	Summary       string    `json:"summary"`
	Error         string    `json:"error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (d *DB) EnsureProductResearchTable() error {
	_, err := d.pool.Exec(productResearchSchema)
	return err
}

func (d *DB) CreateProductResearch(ctx context.Context, j *ProductResearchJob) error {
	_, err := d.pool.ExecContext(ctx, `INSERT INTO product_research_jobs (id,org_id,product_id,website_url,base_knowledge,status,extracted,summary,error_message) VALUES (?,?,?,?,?,'queued',?,'','')`, j.ID, j.OrgID, j.ProductID, j.URL, j.BaseKnowledge, j.Extracted)
	return err
}

func scanResearch(row interface{ Scan(...any) error }) (*ProductResearchJob, error) {
	var j ProductResearchJob
	err := row.Scan(&j.ID, &j.OrgID, &j.ProductID, &j.URL, &j.BaseKnowledge, &j.Status, &j.Extracted, &j.Summary, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}

const researchColumns = `id,org_id,product_id,website_url,base_knowledge,status,extracted,summary,error_message,created_at,updated_at`

func (d *DB) GetProductResearch(ctx context.Context, orgID, productID int64, id string) (*ProductResearchJob, error) {
	return scanResearch(d.pool.QueryRowContext(ctx, `SELECT `+researchColumns+` FROM product_research_jobs WHERE org_id=? AND product_id=? AND id=?`, orgID, productID, id))
}
func (d *DB) LatestProductResearch(ctx context.Context, orgID, productID int64) (*ProductResearchJob, error) {
	return scanResearch(d.pool.QueryRowContext(ctx, `SELECT `+researchColumns+` FROM product_research_jobs WHERE org_id=? AND product_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, orgID, productID))
}
func (d *DB) UpdateProductResearch(ctx context.Context, j *ProductResearchJob) error {
	result, err := d.pool.ExecContext(ctx, `UPDATE product_research_jobs SET status=?,extracted=?,summary=?,error_message=? WHERE id=? AND org_id=? AND product_id=?`, j.Status, j.Extracted, j.Summary, j.Error, j.ID, j.OrgID, j.ProductID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

var ErrResearchConflict = errors.New("Product knowledge or website changed since research started. Refresh the research before applying")

func (d *DB) ApplyProductResearch(ctx context.Context, j *ProductResearchJob, imagesJSON string) error {
	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, summary string
	err = tx.QueryRowContext(ctx, `SELECT status,summary FROM product_research_jobs WHERE id=? AND org_id=? AND product_id=? FOR UPDATE`, j.ID, j.OrgID, j.ProductID).Scan(&state, &summary)
	if err != nil {
		return err
	}
	if state == "applied" {
		return nil
	}
	if state != "ready" {
		return ErrResearchConflict
	}
	var current, website string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(scraped_info,''),COALESCE(website_url,'') FROM products WHERE id=? AND org_id=? FOR UPDATE`, j.ProductID, j.OrgID).Scan(&current, &website)
	if err != nil {
		return err
	}
	if current != j.BaseKnowledge || website != j.URL {
		return ErrResearchConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE products SET scraped_info=?,image_urls=? WHERE id=? AND org_id=?`, summary, imagesJSON, j.ProductID, j.OrgID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE product_research_jobs SET status='applied' WHERE id=? AND org_id=? AND product_id=?`, j.ID, j.OrgID, j.ProductID); err != nil {
		return err
	}
	return tx.Commit()
}
