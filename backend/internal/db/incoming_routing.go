package db

import (
	"context"
	"database/sql"
	"errors"
	"github.com/go-sql-driver/mysql"
)

var ErrForwardingInUse = errors.New("number is selected by an incoming route; switch the route to AI first")

type ForwardingNumber struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}
type IncomingRoute struct {
	Mode          string `json:"mode"`
	DestinationID int64  `json:"destination_id"`
	Phone         string `json:"-"`
	Name          string `json:"-"`
}

func (d *DB) EnsureIncomingRoutingTables() error {
	for _, statement := range IncomingRoutingSchema {
		if _, err := d.pool.Exec(statement); err != nil {
			return err
		}
	}
	_, err := d.pool.Exec("ALTER TABLE incoming_route_snapshots ADD COLUMN destination_name VARCHAR(100) NOT NULL DEFAULT ''")
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1060 {
			return err
		}
	}
	return nil
}

var IncomingRoutingSchema = []string{
	`CREATE TABLE IF NOT EXISTS org_forwarding_numbers (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, org_id BIGINT NOT NULL, name VARCHAR(100) NOT NULL, phone VARCHAR(20) NOT NULL, UNIQUE KEY org_phone (org_id,phone))`,
	`CREATE TABLE IF NOT EXISTS org_incoming_routes (account_id BIGINT NOT NULL PRIMARY KEY, org_id BIGINT NOT NULL, mode VARCHAR(16) NOT NULL DEFAULT 'ai', destination_id BIGINT NOT NULL DEFAULT 0, KEY org_destination (org_id,destination_id))`,
	`CREATE TABLE IF NOT EXISTS incoming_route_snapshots (org_id BIGINT NOT NULL, call_sid VARCHAR(160) NOT NULL, mode VARCHAR(16) NOT NULL, phone VARCHAR(20) NOT NULL DEFAULT '', PRIMARY KEY(org_id,call_sid))`,
}

func (d *DB) ListForwardingNumbers(orgID int64) ([]ForwardingNumber, error) {
	rows, err := d.pool.Query("SELECT id,name,phone FROM org_forwarding_numbers WHERE org_id=? ORDER BY name,id", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ForwardingNumber{}
	for rows.Next() {
		var n ForwardingNumber
		if err := rows.Scan(&n.ID, &n.Name, &n.Phone); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}
func (d *DB) SaveForwardingNumber(orgID int64, n ForwardingNumber) error {
	if n.ID == 0 {
		_, err := d.pool.Exec("INSERT INTO org_forwarding_numbers (org_id,name,phone) VALUES (?,?,?)", orgID, n.Name, n.Phone)
		return err
	}
	result, err := d.pool.Exec("UPDATE org_forwarding_numbers SET name=?,phone=? WHERE id=? AND org_id=?", n.Name, n.Phone, n.ID, orgID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 { // MySQL reports zero for an unchanged row too.
		var id int64
		return d.pool.QueryRow("SELECT id FROM org_forwarding_numbers WHERE id=? AND org_id=?", n.ID, orgID).Scan(&id)
	}
	return nil
}
func (d *DB) DeleteForwardingNumber(orgID, id int64) error {
	tx, err := d.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int64
	if err = tx.QueryRow("SELECT id FROM org_forwarding_numbers WHERE org_id=? AND id=? FOR UPDATE", orgID, id).Scan(&locked); err != nil {
		return err
	}
	var used int
	if err = tx.QueryRow("SELECT COUNT(*) FROM org_incoming_routes WHERE org_id=? AND destination_id=? AND mode='phone'", orgID, id).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return ErrForwardingInUse
	}
	if _, err = tx.Exec("DELETE FROM org_forwarding_numbers WHERE org_id=? AND id=?", orgID, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) GetIncomingRoute(orgID, accountID int64) (IncomingRoute, error) {
	route := IncomingRoute{Mode: "ai"}
	err := d.pool.QueryRow(`SELECT r.mode,r.destination_id,COALESCE(n.phone,''),COALESCE(n.name,'') FROM org_incoming_routes r LEFT JOIN org_forwarding_numbers n ON n.id=r.destination_id AND n.org_id=r.org_id WHERE r.org_id=? AND r.account_id=?`, orgID, accountID).Scan(&route.Mode, &route.DestinationID, &route.Phone, &route.Name)
	if err == sql.ErrNoRows {
		return IncomingRoute{Mode: "ai"}, nil
	}
	return route, err
}
func (d *DB) SaveIncomingRoute(orgID, accountID int64, route IncomingRoute) error {
	tx, err := d.pool.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if route.Mode == "phone" {
		var id int64
		if err = tx.QueryRow("SELECT id FROM org_forwarding_numbers WHERE org_id=? AND id=? FOR UPDATE", orgID, route.DestinationID).Scan(&id); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO org_incoming_routes (account_id,org_id,mode,destination_id) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE mode=VALUES(mode),destination_id=VALUES(destination_id)`, accountID, orgID, route.Mode, route.DestinationID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Persist the first routing decision so webhook retries cannot change destination.
func (d *DB) SnapshotIncomingRoute(ctx context.Context, orgID int64, callSID string, route IncomingRoute) (IncomingRoute, error) {
	_, err := d.pool.ExecContext(ctx, `INSERT INTO incoming_route_snapshots (org_id,call_sid,mode,phone,destination_name) VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE call_sid=VALUES(call_sid)`, orgID, callSID, route.Mode, route.Phone, route.Name)
	if err != nil {
		return route, err
	}
	err = d.pool.QueryRowContext(ctx, "SELECT mode,phone,destination_name FROM incoming_route_snapshots WHERE org_id=? AND call_sid=?", orgID, callSID).Scan(&route.Mode, &route.Phone, &route.Name)
	return route, err
}

func (d *DB) GetIncomingRouteSnapshot(ctx context.Context, orgID int64, callSID string) (IncomingRoute, error) {
	var route IncomingRoute
	err := d.pool.QueryRowContext(ctx, "SELECT mode,phone,destination_name FROM incoming_route_snapshots WHERE org_id=? AND call_sid=?", orgID, callSID).Scan(&route.Mode, &route.Phone, &route.Name)
	return route, err
}
