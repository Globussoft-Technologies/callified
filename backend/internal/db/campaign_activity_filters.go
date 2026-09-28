package db

import "strings"

// CampaignActivityFilter is shared by campaign call logs, reviews, insights,
// and retries so the common dashboard search/date controls behave the same on
// every tab.
type CampaignActivityFilter struct {
	Search string
	From   string
	To     string
}

// campaignActivityFilterClause builds filters against the joined leads alias
// `l` and the caller-provided activity timestamp column. timestampColumn is
// always an internal SQL identifier supplied by the repository code, never
// user input. All user values remain query parameters.
func campaignActivityFilterClause(filter CampaignActivityFilter, timestampColumn string) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 7)

	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		clauses = append(clauses, `(CONCAT_WS(' ', l.first_name, l.last_name) LIKE ? OR l.phone LIKE ? OR l.company LIKE ? OR l.source LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if filter.From != "" {
		clauses = append(clauses, timestampColumn+` >= ?`)
		args = append(args, filter.From)
	}
	if filter.To != "" {
		clauses = append(clauses, timestampColumn+` <= ?`)
		args = append(args, filter.To)
	}

	return strings.Join(clauses, " AND "), args
}
