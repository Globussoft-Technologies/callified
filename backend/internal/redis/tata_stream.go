package redis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TataStreamContext contains only server-verified ownership and call details.
type TataStreamContext struct {
	CallID    string          `json:"call_id"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	AccountID int64           `json:"account_id"`
	Inbound   bool            `json:"inbound"`
	Pending   PendingCallInfo `json:"pending"`
}

func streamTicketKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return key("tata-ticket", hex.EncodeToString(h[:]))
}
func (s *Store) IssueTataStreamTicket(ctx context.Context, info TataStreamContext) (string, error) {
	if s == nil || s.rdb == nil {
		return "", fmt.Errorf("stream authentication requires Redis")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	data, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	claim := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", info.AccountID, info.CallID)))
	const issue = `local existing = redis.call('GET', KEYS[1])
 if existing then
   local old = cjson.decode(existing)
   if redis.call('EXISTS', old.ticket_key) == 1 then return old.token end
   return ''
 end
 if ARGV[3] ~= '' then
   local raw = redis.call('GET', KEYS[3])
   if not raw then return '' end
   local pending = cjson.decode(raw)
   if pending.tata_reservation_id ~= ARGV[3] then return '' end
   if pending.tata_stream_call_id and pending.tata_stream_call_id ~= ARGV[4] then return '' end
   pending.tata_stream_call_id = ARGV[4]
   redis.call('SET', KEYS[3], cjson.encode(pending), 'KEEPTTL')
 end
 redis.call('SET', KEYS[2], ARGV[1], 'EX', 45)
 redis.call('SET', KEYS[1], ARGV[2], 'EX', 300)
 return ARGV[5]`
	ticketKey := streamTicketKey(token)
	cached, _ := json.Marshal(map[string]string{"token": token, "ticket_key": ticketKey})
	reservation := ""
	if !info.Inbound {
		reservation = info.Pending.TataReservationID
		if reservation == "" {
			return "", fmt.Errorf("missing reservation identity")
		}
	}
	issued, err := s.rdb.Eval(ctx, issue, []string{key("tata-issued", hex.EncodeToString(claim[:])), ticketKey, tataDialKey(info.AccountID, info.Pending.Phone)}, data, cached, reservation, info.CallID, token).Text()
	if err != nil || issued == "" {
		return "", fmt.Errorf("cannot issue stream ticket")
	}
	token = issued
	return token, nil
}
func (s *Store) ConsumeTataStreamTicket(ctx context.Context, token string) (*TataStreamContext, error) {
	if s == nil || s.rdb == nil || len(token) != 64 {
		return nil, fmt.Errorf("invalid or unavailable stream ticket")
	}
	data, err := s.rdb.GetDel(ctx, streamTicketKey(token)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("invalid or expired stream ticket")
	}
	var info TataStreamContext
	if json.Unmarshal(data, &info) != nil || info.Pending.OrgID <= 0 || info.AccountID <= 0 || info.CallID == "" {
		return nil, fmt.Errorf("invalid stream context")
	}
	if !info.Inbound {
		s.releaseTataReservation(ctx, info.Pending)
	}
	return &info, nil
}

// NormalizeTataNumber canonicalizes E.164 formatting without guessing country codes.
func NormalizeTataNumber(v string) string {
	return strings.TrimPrefix(strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(v)), "+")
}
func tataDialKey(account int64, phone string) string {
	number := NormalizeTataNumber(phone)
	// Match the Indian domestic normalization used by dial.NormalizePhone.
	// This applies only to pending outbound destinations, not account DID checks.
	if strings.HasPrefix(number, "0") {
		number = strings.TrimPrefix(number, "0")
	}
	if len(number) == 10 {
		number = "91" + number
	}
	return key("tata-dial", fmt.Sprintf("%d:%s", account, number))
}

// ReserveTataDial rejects overlapping calls to the same destination/account.
func (s *Store) ReserveTataDial(ctx context.Context, info *PendingCallInfo) error {
	if info.ExotelAccountID <= 0 || info.OrgID <= 0 {
		return fmt.Errorf("Tata streaming requires a saved organization provider account")
	}
	if s == nil || s.rdb == nil {
		return fmt.Errorf("Tata streaming requires Redis")
	}
	if info.TataReservationID == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		info.TataReservationID = hex.EncodeToString(b)
	}
	data, _ := json.Marshal(info)
	ok, err := s.rdb.SetNX(ctx, tataDialKey(info.ExotelAccountID, info.Phone), data, 2*time.Minute).Result()
	if err != nil || !ok {
		return fmt.Errorf("Tata destination already pending or Redis unavailable")
	}
	return nil
}

// GetTataDial reads without consuming; ticket creation binds the reservation atomically.
func (s *Store) GetTataDial(ctx context.Context, account int64, phone string) (PendingCallInfo, error) {
	if s == nil || s.rdb == nil {
		return PendingCallInfo{}, fmt.Errorf("Redis unavailable")
	}
	data, err := s.rdb.Get(ctx, tataDialKey(account, phone)).Bytes()
	var info PendingCallInfo
	if err != nil || json.Unmarshal(data, &info) != nil || info.ExotelAccountID != account {
		return info, fmt.Errorf("no matching pending call")
	}
	return info, nil
}
func (s *Store) releaseTataReservation(ctx context.Context, info PendingCallInfo) {
	if s == nil || s.rdb == nil || info.TataReservationID == "" {
		return
	}
	const script = `local raw=redis.call('GET',KEYS[1]);if not raw then return 0 end
 if cjson.decode(raw).tata_reservation_id ~= ARGV[1] then return 0 end
 return redis.call('DEL',KEYS[1])`
	s.rdb.Eval(ctx, script, []string{tataDialKey(info.ExotelAccountID, info.Phone)}, info.TataReservationID)
}
func (s *Store) ReleaseTataDial(ctx context.Context, info PendingCallInfo) {
	s.releaseTataReservation(ctx, info)
}
func (s *Store) UpdateTataDial(ctx context.Context, info PendingCallInfo) {
	if s == nil || s.rdb == nil || info.TataReservationID == "" {
		return
	}
	const update = `local raw=redis.call('GET',KEYS[1]);if not raw then return 0 end
 local old=cjson.decode(raw);if old.tata_reservation_id ~= ARGV[1] then return 0 end
 local new=cjson.decode(ARGV[2]);new.tata_stream_call_id=old.tata_stream_call_id
 redis.call('SET',KEYS[1],cjson.encode(new),'KEEPTTL');return 1`
	data, _ := json.Marshal(info)
	s.rdb.Eval(ctx, update, []string{tataDialKey(info.ExotelAccountID, info.Phone)}, info.TataReservationID, data)
}

// ReleaseTataDialForCall cannot delete a newer dial's reservation on a late webhook.
func (s *Store) ReleaseTataDialForCall(ctx context.Context, info PendingCallInfo, callID string) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("Redis unavailable")
	}
	const release = `local data = redis.call('GET', KEYS[1])
 if not data then return 0 end
 local pending = cjson.decode(data)
 if pending.exotel_call_sid ~= ARGV[1] then return 0 end
 return redis.call('DEL', KEYS[1])`
	return s.rdb.Eval(ctx, release, []string{tataDialKey(info.ExotelAccountID, info.Phone)}, callID).Err()
}
