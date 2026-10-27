package db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyResearchTransaction(t *testing.T) {
	for _, tc := range []struct {
		name, status, current, website string
		conflict                       bool
	}{
		{"apply", "ready", "approved old", "https://example.com", false},
		{"stale knowledge", "ready", "new human approved", "https://example.com", true},
		{"changed URL", "ready", "approved old", "https://other.example", true},
		{"not ready", "failed", "", "", true},
		{"already applied", "applied", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			d := &DB{pool: pool}
			j := &ProductResearchJob{ID: "job", OrgID: 8, ProductID: 9, URL: "https://example.com", BaseKnowledge: "approved old"}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT status,summary FROM product_research_jobs").WithArgs("job", int64(8), int64(9)).WillReturnRows(sqlmock.NewRows([]string{"status", "summary"}).AddRow(tc.status, "preview"))
			if tc.status == "ready" {
				mock.ExpectQuery("SELECT COALESCE\\(scraped_info").WithArgs(int64(9), int64(8)).WillReturnRows(sqlmock.NewRows([]string{"scraped_info", "website_url"}).AddRow(tc.current, tc.website))
			}
			if tc.conflict || tc.status == "applied" {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("UPDATE products SET scraped_info").WithArgs("preview", "[]", int64(9), int64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE product_research_jobs SET status='applied'").WithArgs("job", int64(8), int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = d.ApplyProductResearch(context.Background(), j, "[]")
			if tc.conflict && err != ErrResearchConflict {
				t.Fatalf("expected conflict got %v", err)
			}
			if !tc.conflict && err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetResearchTenantScope(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	d := &DB{pool: pool}
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT "+researchColumns+" FROM product_research_jobs WHERE org_id=? AND product_id=? AND id=?")).WithArgs(int64(3), int64(4), "opaque-job").WillReturnRows(sqlmock.NewRows([]string{"id", "org_id", "product_id", "website_url", "base_knowledge", "status", "extracted", "summary", "error_message", "created_at", "updated_at"}).AddRow("opaque-job", 3, 4, "https://example.com", "old", "ready", "{}", "new", "", now, now))
	job, err := d.GetProductResearch(context.Background(), 3, 4, "opaque-job")
	if err != nil || job.OrgID != 3 || job.ProductID != 4 {
		t.Fatalf("job %v error %v", job, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyResearchRollsBackIfJobUpdateFails(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	d := &DB{pool: pool}
	job := &ProductResearchJob{ID: "job", OrgID: 8, ProductID: 9, URL: "https://example.com", BaseKnowledge: "old"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status,summary FROM product_research_jobs").WithArgs("job", int64(8), int64(9)).WillReturnRows(sqlmock.NewRows([]string{"status", "summary"}).AddRow("ready", "new"))
	mock.ExpectQuery("SELECT COALESCE\\(scraped_info").WithArgs(int64(9), int64(8)).WillReturnRows(sqlmock.NewRows([]string{"scraped_info", "website_url"}).AddRow("old", "https://example.com"))
	mock.ExpectExec("UPDATE products SET scraped_info").WithArgs("new", "[]", int64(9), int64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE product_research_jobs SET status='applied'").WithArgs("job", int64(8), int64(9)).WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	if err := d.ApplyProductResearch(context.Background(), job, "[]"); err == nil {
		t.Fatal("expected failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProductRemovesResearchAtomically(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM product_research_jobs WHERE product_id").WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM products WHERE id").WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := d.DeleteProduct(9); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
