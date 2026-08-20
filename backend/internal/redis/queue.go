package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/globussoft/callified-backend/internal/metrics"
)

// DialJob is the payload stored in the Redis-backed dial queue.
type DialJob struct {
	LeadID          int64     `json:"lead_id"`
	LeadName        string    `json:"lead_name"`
	LeadPhone       string    `json:"lead_phone"`
	CampaignID      int64     `json:"campaign_id"`
	OrgID           int64     `json:"org_id"`
	Interest        string    `json:"interest"`
	TTSProvider     string    `json:"tts_provider"`
	TTSVoiceID      string    `json:"tts_voice_id"`
	TTSLanguage     string    `json:"tts_language"`
	UserEmail       string    `json:"user_email"`
	UserID          int64     `json:"user_id"`
	ExotelAccountID int64     `json:"exotel_account_id"`
	Attempt         int       `json:"attempt"`
	MaxAttempts     int       `json:"max_attempts"`
	EnqueuedAt      time.Time `json:"enqueued_at"`
	LastError       string    `json:"last_error,omitempty"`
}

// DialState tracks the progress of a queued auto-dial run for one campaign.
type DialState struct {
	CampaignID     int64     `json:"campaign_id"`
	Running        bool      `json:"running"`
	Paused         bool      `json:"paused"`
	Aborted        bool      `json:"aborted"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	QueuedCount    int       `json:"queued_count"`
	ProcessedCount int       `json:"processed_count"`
	FailedCount    int       `json:"failed_count"`
	RetryCount     int       `json:"retry_count"`
	LastError      string    `json:"last_error,omitempty"`
}

const dialQueueKey = "dial_queue"
const dialRetryKey = "dial_retry"

func dialStateKey(campaignID int64) string {
	return fmt.Sprintf("campaign:%d:dial_state", campaignID)
}

// EnqueueDialJobs pushes jobs to the tail of the global dial queue and updates
// the per-campaign state atomically. If Redis is unavailable, jobs are silently
// dropped — callers must handle the returned error.
func (s *Store) EnqueueDialJobs(ctx context.Context, state DialState, jobs []DialJob) error {
	if s.rdb == nil {
		return fmt.Errorf("redis unavailable: cannot enqueue dial jobs")
	}
	pipe := s.rdb.TxPipeline()
	for _, j := range jobs {
		b, err := json.Marshal(j)
		if err != nil {
			return fmt.Errorf("marshal dial job: %w", err)
		}
		pipe.RPush(ctx, key(dialQueueKey), string(b))
	}
	stateBytes, _ := json.Marshal(state)
	pipe.HSet(ctx, key(dialStateKey(state.CampaignID)), "state", string(stateBytes))
	_, err := pipe.Exec(ctx)
	return err
}

// NextDialJob removes and returns the next job from the head of the dial queue.
// Returns nil when the queue is empty or Redis is unavailable.
func (s *Store) NextDialJob(ctx context.Context) (*DialJob, error) {
	if s.rdb == nil {
		return nil, nil
	}
	val, err := s.rdb.LPop(ctx, key(dialQueueKey)).Result()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var job DialJob
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return nil, fmt.Errorf("unmarshal dial job: %w", err)
	}
	return &job, nil
}

// BlockingNextDialJob blocks up to timeout waiting for a dial job. Returns nil
// on timeout or Redis unavailability.
func (s *Store) BlockingNextDialJob(ctx context.Context, timeout time.Duration) (*DialJob, error) {
	if s.rdb == nil {
		return nil, nil
	}
	val, err := s.rdb.BLPop(ctx, timeout, key(dialQueueKey)).Result()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(val) < 2 {
		return nil, nil
	}
	var job DialJob
	if err := json.Unmarshal([]byte(val[1]), &job); err != nil {
		return nil, fmt.Errorf("unmarshal dial job: %w", err)
	}
	return &job, nil
}

// EnqueueDialRetry schedules a job for retry with the given delay.
func (s *Store) EnqueueDialRetry(ctx context.Context, job *DialJob, delay time.Duration) error {
	if s.rdb == nil {
		return fmt.Errorf("redis unavailable: cannot schedule retry")
	}
	job.Attempt++
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	score := float64(time.Now().Add(delay).Unix())
	return s.rdb.ZAdd(ctx, key(dialRetryKey), goredis.Z{Score: score, Member: string(b)}).Err()
}

// PollDialRetries returns all jobs whose retry time is now or in the past, and
// removes them from the retry set atomically.
func (s *Store) PollDialRetries(ctx context.Context, max int64) ([]*DialJob, error) {
	if s.rdb == nil {
		return nil, nil
	}
	now := float64(time.Now().Unix())
	members, err := s.rdb.ZRangeByScore(ctx, key(dialRetryKey), &goredis.ZRangeBy{
		Min: "-inf", Max: fmt.Sprintf("%.0f", now), Count: max,
	}).Result()
	if err != nil || len(members) == 0 {
		return nil, err
	}
	pipe := s.rdb.TxPipeline()
	for _, m := range members {
		pipe.ZRem(ctx, key(dialRetryKey), m)
		pipe.RPush(ctx, key(dialQueueKey), m)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]*DialJob, 0, len(members))
	for _, m := range members {
		var job DialJob
		if err := json.Unmarshal([]byte(m), &job); err != nil {
			continue
		}
		jobs = append(jobs, &job)
	}
	return jobs, nil
}

// GetDialState returns the current dial state for a campaign, or a zero state
// when none exists.
func (s *Store) GetDialState(ctx context.Context, campaignID int64) (DialState, error) {
	var state DialState
	if s.rdb == nil {
		return state, nil
	}
	val, err := s.rdb.HGet(ctx, key(dialStateKey(campaignID)), "state").Result()
	if err == goredis.Nil {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	_ = json.Unmarshal([]byte(val), &state)
	return state, nil
}

// SetDialState persists the dial state for a campaign.
func (s *Store) SetDialState(ctx context.Context, state DialState) error {
	if s.rdb == nil {
		return fmt.Errorf("redis unavailable: cannot set dial state")
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.rdb.HSet(ctx, key(dialStateKey(state.CampaignID)), "state", string(b)).Err()
}

// PauseDialQueue marks a campaign queue as paused.
func (s *Store) PauseDialQueue(ctx context.Context, campaignID int64) error {
	state, err := s.GetDialState(ctx, campaignID)
	if err != nil {
		return err
	}
	state.Paused = true
	return s.SetDialState(ctx, state)
}

// ResumeDialQueue unpauses a campaign queue.
func (s *Store) ResumeDialQueue(ctx context.Context, campaignID int64) error {
	state, err := s.GetDialState(ctx, campaignID)
	if err != nil {
		return err
	}
	state.Paused = false
	return s.SetDialState(ctx, state)
}

// UpdateQueueDepthMetrics refreshes Prometheus queue-depth gauges for the
// global dial queue, retry queue, and per-campaign dial state counts.
func (s *Store) UpdateQueueDepthMetrics(ctx context.Context) {
	if s.rdb == nil {
		metrics.QueueDepth.WithLabelValues("dial").Set(0)
		metrics.QueueDepth.WithLabelValues("retry").Set(0)
		return
	}
	if n, err := s.rdb.LLen(ctx, key(dialQueueKey)).Result(); err == nil {
		metrics.QueueDepth.WithLabelValues("dial").Set(float64(n))
	}
	if n, err := s.rdb.ZCard(ctx, key(dialRetryKey)).Result(); err == nil {
		metrics.QueueDepth.WithLabelValues("retry").Set(float64(n))
	}
}

// AbortDialQueue marks a campaign queue as aborted and drains remaining jobs.
func (s *Store) AbortDialQueue(ctx context.Context, campaignID int64) error {
	state, err := s.GetDialState(ctx, campaignID)
	if err != nil {
		return err
	}
	state.Aborted = true
	state.Running = false
	state.Paused = false
	state.CompletedAt = time.Now()
	state.LastError = "aborted by user"
	if err := s.SetDialState(ctx, state); err != nil {
		return err
	}
	// Drain any remaining jobs for this campaign from the main queue.
	// Best-effort: re-read the queue, filter out matching jobs, and re-push.
	for {
		val, err := s.rdb.LPop(ctx, key(dialQueueKey)).Result()
		if err == goredis.Nil {
			break
		}
		if err != nil {
			return err
		}
		var job DialJob
		if err := json.Unmarshal([]byte(val), &job); err != nil {
			continue
		}
		if job.CampaignID != campaignID {
			_ = s.rdb.RPush(ctx, key(dialQueueKey), val).Err()
		}
	}
	return nil
}
