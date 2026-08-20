package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// PromptTemplate mirrors the prompt_templates table.
type PromptTemplate struct {
	ID             int64  `json:"id"`
	OrgID          int64  `json:"org_id"`
	Name           string `json:"name"`
	Language       string `json:"language"`
	TemplateType   string `json:"template_type"`
	ScriptBody     string `json:"script_body"`
	Version        int    `json:"version"`
	IsActive       bool   `json:"is_active"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// EnsurePromptTemplatesTable creates the prompt_templates table if it doesn't
// exist and adds any columns that may be missing on legacy schemas.
func (d *DB) EnsurePromptTemplatesTable() error {
	_, err := d.pool.Exec(`
		CREATE TABLE IF NOT EXISTS prompt_templates (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			org_id BIGINT DEFAULT NULL,
			name VARCHAR(255) NOT NULL,
			language VARCHAR(10) DEFAULT 'en',
			template_type VARCHAR(50) DEFAULT 'voice',
			script_body LONGTEXT NOT NULL,
			version INT NOT NULL DEFAULT 1,
			is_active TINYINT(1) NOT NULL DEFAULT 1,
			created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_org_lang (org_id, language),
			INDEX idx_name (name),
			INDEX idx_active (is_active),
			FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`)
	if err != nil {
		return err
	}
	columns := []struct{ name, def string }{
		{"language", "VARCHAR(10) DEFAULT 'en'"},
		{"template_type", "VARCHAR(50) DEFAULT 'voice'"},
		{"version", "INT NOT NULL DEFAULT 1"},
		{"is_active", "TINYINT(1) NOT NULL DEFAULT 1"},
		{"updated_at", "TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"},
	}
	for _, col := range columns {
		_, alterErr := d.pool.Exec(fmt.Sprintf("ALTER TABLE prompt_templates ADD COLUMN %s %s", col.name, col.def))
		if alterErr != nil && !strings.Contains(alterErr.Error(), "Duplicate column name") {
			return alterErr
		}
	}
	// Add opening_script_id columns to products and campaigns if missing.
	_, _ = d.pool.Exec(`ALTER TABLE products ADD COLUMN opening_script_id BIGINT DEFAULT NULL`)
	_, _ = d.pool.Exec(`ALTER TABLE campaigns ADD COLUMN opening_script_id BIGINT DEFAULT NULL`)
	return nil
}

const promptTemplateCols = `id, org_id, name, COALESCE(language,'en'), COALESCE(template_type,'voice'),
	script_body, version, is_active, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s'), DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s')`

func scanPromptTemplate(row interface{ Scan(...any) error }) (*PromptTemplate, error) {
	t := &PromptTemplate{}
	var orgID sql.NullInt64
	var isActive int
	err := row.Scan(&t.ID, &orgID, &t.Name, &t.Language, &t.TemplateType, &t.ScriptBody, &t.Version, &isActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if orgID.Valid {
		t.OrgID = orgID.Int64
	}
	t.IsActive = isActive == 1
	return t, nil
}

// GetPromptTemplateByID fetches one template by ID.
func (d *DB) GetPromptTemplateByID(id int64) (*PromptTemplate, error) {
	row := d.pool.QueryRow(`SELECT `+promptTemplateCols+` FROM prompt_templates WHERE id=?`, id)
	t, err := scanPromptTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// GetPromptTemplateByOrgName fetches an org-scoped template by name and language.
func (d *DB) GetPromptTemplateByOrgName(orgID int64, name, language string) (*PromptTemplate, error) {
	row := d.pool.QueryRow(`SELECT `+promptTemplateCols+` FROM prompt_templates WHERE org_id=? AND name=? AND language=? AND is_active=1 ORDER BY version DESC LIMIT 1`, orgID, name, language)
	t, err := scanPromptTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// GetDefaultPromptTemplate fetches the global default template for a language.
func (d *DB) GetDefaultPromptTemplate(language string) (*PromptTemplate, error) {
	row := d.pool.QueryRow(`SELECT `+promptTemplateCols+` FROM prompt_templates WHERE org_id IS NULL AND name=? AND is_active=1 ORDER BY version DESC LIMIT 1`, defaultTemplateName(language))
	t, err := scanPromptTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// ListPromptTemplates returns active templates for an org plus global defaults.
func (d *DB) ListPromptTemplates(orgID int64) ([]PromptTemplate, error) {
	rows, err := d.pool.Query(`
		SELECT `+promptTemplateCols+`
		FROM prompt_templates
		WHERE is_active=1 AND (org_id=? OR org_id IS NULL)
		ORDER BY org_id IS NULL, name, version DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PromptTemplate
	for rows.Next() {
		t, err := scanPromptTemplate(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *t)
	}
	return list, rows.Err()
}

// CreatePromptTemplate inserts a new template.
func (d *DB) CreatePromptTemplate(orgID int64, name, language, templateType, scriptBody string) (int64, error) {
	res, err := d.pool.Exec(
		`INSERT INTO prompt_templates (org_id, name, language, template_type, script_body, version) VALUES (?,?,?,?,?,1)`,
		nullInt64(orgID), name, language, templateType, scriptBody)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdatePromptTemplate inserts a new version of a template, marking older versions inactive.
func (d *DB) UpdatePromptTemplate(id int64, orgID int64, scriptBody string) (int64, error) {
	var name, language, templateType string
	var version int
	err := d.pool.QueryRow(`SELECT name, language, template_type, version FROM prompt_templates WHERE id=?`, id).Scan(&name, &language, &templateType, &version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("template not found")
		}
		return 0, err
	}
	// Mark existing versions inactive.
	if _, err := d.pool.Exec(`UPDATE prompt_templates SET is_active=0 WHERE org_id=? AND name=? AND language=?`, nullInt64(orgID), name, language); err != nil {
		return 0, err
	}
	res, err := d.pool.Exec(
		`INSERT INTO prompt_templates (org_id, name, language, template_type, script_body, version) VALUES (?,?,?,?,?,?)`,
		nullInt64(orgID), name, language, templateType, scriptBody, version+1)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeactivatePromptTemplate marks a template and its versions inactive.
func (d *DB) DeactivatePromptTemplate(id int64) error {
	_, err := d.pool.Exec(`UPDATE prompt_templates SET is_active=0 WHERE id=?`, id)
	return err
}

func defaultTemplateName(language string) string {
	return "default_voice_" + language
}
