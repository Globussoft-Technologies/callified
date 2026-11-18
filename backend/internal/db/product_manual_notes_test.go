package db

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestManualNotesCanBeDisabledWithoutDeletingText(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		p := Product{ManualNotes: "Saved reference", UseManualNotes: &enabled}
		require.Equal(t, enabled, p.ManualNotesEnabled())
		if enabled {
			require.Equal(t, "Saved reference", p.EnabledManualNotes())
		} else {
			require.Empty(t, p.EnabledManualNotes())
		}
		require.Equal(t, "Saved reference", p.ManualNotes)
	}
	require.Equal(t, "Legacy notes", (Product{ManualNotes: "Legacy notes"}).EnabledManualNotes())
}

func TestManualNotesSettingPersistsBothStatesAndPreservesOmittedValue(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	for _, enabled := range []bool{true, false} {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE products SET agent_persona=?, call_flow_instructions=?, use_manual_notes=? WHERE id=?")).WithArgs("Persona", "Flow", enabled, int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, d.UpdateProductPromptSettings(5, "Persona", "Flow", nil, &enabled))
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE products SET agent_persona=?, call_flow_instructions=? WHERE id=?")).WithArgs("Persona", "Flow", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.UpdateProductPromptSettings(5, "Persona", "Flow", nil, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}
