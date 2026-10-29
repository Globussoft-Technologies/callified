package db

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUpdateProductPromptOptionalIdentity(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	opening, agent := "Hi {{agent_name}}", " Meera "
	mock.ExpectExec(`UPDATE products SET agent_persona=\?, call_flow_instructions=\?, opening_greeting=\?, agent_name=\? WHERE id=\?`).
		WithArgs("Persona", "Flow", opening, "Meera", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.UpdateProductPrompt(5, "Persona", "Flow", &opening, &agent))
	// Persona generation and older clients do not clear the custom name or opening.
	mock.ExpectExec(`UPDATE products SET agent_persona=\?, call_flow_instructions=\? WHERE id=\?`).
		WithArgs("Persona", "Flow", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.UpdateProductPrompt(5, "Persona", "Flow"))
	blank := ""
	mock.ExpectExec(`UPDATE products SET agent_persona=\?, call_flow_instructions=\?, agent_name=\? WHERE id=\?`).
		WithArgs("Persona", "Flow", nil, int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.UpdateProductPrompt(5, "Persona", "Flow", nil, &blank))
	require.NoError(t, mock.ExpectationsWereMet())
}
