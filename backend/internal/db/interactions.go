package db

import (
	"fmt"
	"sort"
	"time"
)

// InteractionEventType categorizes one row in the customer timeline.
type InteractionEventType string

const (
	InteractionLeadCreated   InteractionEventType = "lead_created"
	InteractionNote          InteractionEventType = "note"
	InteractionCall          InteractionEventType = "call"
	InteractionScheduledCall InteractionEventType = "scheduled_call"
	InteractionWhatsApp      InteractionEventType = "whatsapp"
	InteractionStatusChange  InteractionEventType = "status_change"
)

// InteractionEvent is a single item in the unified customer timeline.
type InteractionEvent struct {
	ID        int64                `json:"id"`
	Type      InteractionEventType `json:"type"`
	Timestamp string               `json:"timestamp"`
	Title     string               `json:"title"`
	Body      string               `json:"body"`
	Metadata  map[string]any       `json:"metadata"`
}

// InteractionTimeline groups the lead header with all interactions.
type InteractionTimeline struct {
	Lead       Lead               `json:"lead"`
	Events     []InteractionEvent `json:"events"`
	EventCount int                `json:"event_count"`
}

// GetLeadInteractions builds a complete interaction timeline for a single lead.
// It merges lead creation, notes, calls, scheduled calls, and WhatsApp messages
// into one chronological feed.
func (d *DB) GetLeadInteractions(orgID, leadID int64) (*InteractionTimeline, error) {
	lead, err := d.GetLeadByID(leadID)
	if err != nil {
		return nil, err
	}
	if lead == nil || lead.OrgID != orgID {
		return nil, nil
	}

	var events []InteractionEvent

	// Lead creation.
	if lead.CreatedAt != "" {
		events = append(events, InteractionEvent{
			Type:      InteractionLeadCreated,
			Timestamp: lead.CreatedAt,
			Title:     "Lead created",
			Body:      fmt.Sprintf("Added from %s", lead.Source),
			Metadata: map[string]any{
				"source": lead.Source,
				"status": lead.Status,
			},
		})
	}

	// Latest note/follow-up note.
	if lead.FollowUpNote != "" {
		ts := lead.CreatedAt
		if ts == "" {
			ts = time.Now().UTC().Format("2006-01-02T15:04:05Z")
		}
		events = append(events, InteractionEvent{
			Type:      InteractionNote,
			Timestamp: ts,
			Title:     "Follow-up note",
			Body:      lead.FollowUpNote,
			Metadata:  map[string]any{},
		})
	}

	// Current status snapshot (we don't have an audit log, so surface the latest status).
	if lead.Status != "" {
		ts := lead.CreatedAt
		if ts == "" {
			ts = time.Now().UTC().Format("2006-01-02T15:04:05Z")
		}
		events = append(events, InteractionEvent{
			Type:      InteractionStatusChange,
			Timestamp: ts,
			Title:     "Status",
			Body:      lead.Status,
			Metadata: map[string]any{
				"status": lead.Status,
			},
		})
	}

	// Calls from call_transcripts.
	transcripts, err := d.GetTranscriptsByLead(leadID)
	if err != nil {
		return nil, err
	}
	for _, t := range transcripts {
		outcome := "No Answer"
		if t.CallDurationS > 30 {
			outcome = "Completed"
		} else if t.CallDurationS > 5 {
			outcome = "Connected"
		}
		events = append(events, InteractionEvent{
			ID:        t.ID,
			Type:      InteractionCall,
			Timestamp: t.CreatedAt,
			Title:     "Call",
			Body:      outcome,
			Metadata: map[string]any{
				"duration_s":    t.CallDurationS,
				"recording_url": t.RecordingURL,
				"tts_language":  t.TTSLanguage,
				"outcome":       outcome,
			},
		})
	}

	// Scheduled calls.
	scheduledRows, err := d.pool.Query(`
		SELECT id, DATE_FORMAT(scheduled_at,'%Y-%m-%dT%H:%i:%sZ'),
		COALESCE(status,'pending'), COALESCE(mode,'ai'), COALESCE(notes,'')
		FROM scheduled_calls
		WHERE lead_id=? ORDER BY scheduled_at DESC`, leadID)
	if err != nil {
		return nil, err
	}
	defer scheduledRows.Close()
	for scheduledRows.Next() {
		var id int64
		var scheduledAt, status, mode, notes string
		if err := scheduledRows.Scan(&id, &scheduledAt, &status, &mode, &notes); err != nil {
			return nil, err
		}
		title := "Scheduled call"
		if mode == "manual" {
			title = "Scheduled callback"
		}
		events = append(events, InteractionEvent{
			ID:        id,
			Type:      InteractionScheduledCall,
			Timestamp: scheduledAt,
			Title:     title,
			Body:      notes,
			Metadata: map[string]any{
				"status": status,
				"mode":   mode,
			},
		})
	}
	if err := scheduledRows.Err(); err != nil {
		return nil, err
	}

	// WhatsApp messages matched by lead phone.
	if lead.Phone != "" {
		waRows, err := d.pool.Query(`
			SELECT m.id, DATE_FORMAT(m.created_at,'%Y-%m-%dT%H:%i:%sZ'),
			COALESCE(m.direction,''), COALESCE(m.message_text,'')
			FROM whatsapp_messages m
			JOIN whatsapp_conversations c ON m.conversation_id=c.id
			WHERE c.org_id=? AND c.phone=? ORDER BY m.created_at DESC`, orgID, lead.Phone)
		if err == nil {
			defer waRows.Close()
			for waRows.Next() {
				var id int64
				var createdAt, direction, text string
				if err := waRows.Scan(&id, &createdAt, &direction, &text); err != nil {
					return nil, err
				}
				title := "WhatsApp outbound"
				if direction == "inbound" {
					title = "WhatsApp inbound"
				}
				events = append(events, InteractionEvent{
					ID:        id,
					Type:      InteractionWhatsApp,
					Timestamp: createdAt,
					Title:     title,
					Body:      text,
					Metadata: map[string]any{
						"direction": direction,
					},
				})
			}
		}
		// If the WA tables don't exist or the query errors, skip WA events rather
		// than failing the whole timeline.
	}

	// Newest first.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp > events[j].Timestamp
	})

	return &InteractionTimeline{
		Lead:       *lead,
		Events:     events,
		EventCount: len(events),
	}, nil
}
