package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

func TestProductFollowUpSettingCanBeEnabledDisabledAndOmitted(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	for _, enabled := range []bool{true, false} {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE products SET agent_persona=?, call_flow_instructions=?, allow_customer_follow_ups=? WHERE id=?")).WithArgs("Persona", "Flow", enabled, int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, d.UpdateProductPromptWithFollowUps(5, "Persona", "Flow", &enabled))
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE products SET agent_persona=?, call_flow_instructions=? WHERE id=?")).WithArgs("Persona", "Flow", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.UpdateProductPromptWithFollowUps(5, "Persona", "Flow", nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProductFollowUpSettingSurvivesRead(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	for _, enabled := range []bool{true, false} {
		rows := sqlmock.NewRows([]string{"id", "org_id", "name", "website_url", "scraped_info", "manual_notes", "agent_persona", "call_flow_instructions", "opening_greeting", "agent_name", "created_at", "image_urls", "manual_images", "allow_customer_follow_ups", "use_manual_notes"}).AddRow(5, 7, "Windows", "", "", "Notes", "Persona", "Flow", "", "", "2026-11-16", "[]", "[]", enabled, true)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT " + productCols + " FROM products WHERE id=?")).WithArgs(int64(5)).WillReturnRows(rows)
		p, err := d.GetProductByID(5)
		require.NoError(t, err)
		require.Equal(t, enabled, p.AllowCustomerFollowUps)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
