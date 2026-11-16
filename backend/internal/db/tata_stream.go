package db

import (
	"context"
	"fmt"
)

func (d *DB) SetTataStreamSecret(id, orgID int64, hash string) error {
	result, err := d.pool.Exec(`UPDATE org_exotel_accounts SET tata_stream_secret_hash=?, tata_stream_verified_hash=NULL WHERE id=? AND org_id=? AND provider IN ('tata','smartflo','tata_tele') AND user_id IS NULL`, hash, id, orgID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("Tata account not found")
	}
	return nil
}
func (d *DB) GetTataAccountByStreamSecret(ctx context.Context, hash string) (*OrgExotelAccount, error) {
	a := &OrgExotelAccount{}
	err := d.pool.QueryRowContext(ctx, `SELECT id,org_id,COALESCE(direction,'outbound'),caller_id,provider FROM org_exotel_accounts WHERE tata_stream_secret_hash=? AND provider IN ('tata','smartflo','tata_tele') AND user_id IS NULL`, hash).Scan(&a.ID, &a.OrgID, &a.Direction, &a.CallerID, &a.Provider)
	return a, err
}

func (d *DB) MarkTataStreamVerified(ctx context.Context, id, orgID int64, hash string) error {
	_, err := d.pool.ExecContext(ctx, `UPDATE org_exotel_accounts SET tata_stream_verified_hash=? WHERE id=? AND org_id=? AND tata_stream_secret_hash=?`, hash, id, orgID, hash)
	if err != nil {
		return err
	}
	var ready bool
	err = d.pool.QueryRowContext(ctx, `SELECT COALESCE(tata_stream_secret_hash=? AND tata_stream_verified_hash=?,FALSE) FROM org_exotel_accounts WHERE id=? AND org_id=?`, hash, hash, id, orgID).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("secret not verified")
	}
	return nil
}
func (d *DB) TataStreamReady(ctx context.Context, id, orgID int64) (bool, error) {
	var ready bool
	err := d.pool.QueryRowContext(ctx, `SELECT COALESCE(tata_stream_secret_hash <> '' AND tata_stream_verified_hash=tata_stream_secret_hash,FALSE) FROM org_exotel_accounts WHERE id=? AND org_id=? AND provider IN ('tata','smartflo','tata_tele')`, id, orgID).Scan(&ready)
	return ready, err
}
