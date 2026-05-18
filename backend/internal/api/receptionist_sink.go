package api

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// receptionistSink bridges wsphone-completed calls into the dashboard's
// call_transcripts table so the lead's Past Conversations modal renders
// receptionist calls alongside campaign calls.
//
// Implements wsphone.PastConversationSink. Wired in via
// receptionist_proxy.go when *Server has a non-nil db.
type receptionistSink struct {
	s *Server
}

// SaveReceptionistCall is fire-and-forget. The caller (wsphone.cleanup)
// must not block on the dashboard write — if we crash, the receptionist's
// own filesystem-backed recordings.Store still has the audio.
//
// Persistence steps:
//  1. Look up the lead by phone (the receptionist had to have a prior
//     call_transcripts row for HasPriorCallByPhone to route the caller
//     here in the first place, so the lead exists).
//  2. Write the stereo WAV to RECORDINGS_DIR with a deterministic
//     filename so `/api/recordings/{filename}` can stream it back.
//  3. Insert a row into call_transcripts (campaign_id=0 since this is
//     not a campaign call — Past Conversations lists transcripts by
//     lead_id, so campaign_id can be NULL without breaking the view).
//
// Each step is independently logged so a partial failure still gives
// us breadcrumbs to debug from.
func (rs *receptionistSink) SaveReceptionistCall(
	phone, callSid string,
	wavBytes []byte,
	transcriptJSON, language string,
	durationS float32,
) {
	if rs == nil || rs.s == nil || rs.s.db == nil {
		return
	}
	log := rs.s.logger.Sugar().With(
		"phone", phone,
		"call_sid", callSid,
		"duration_s", durationS,
		"wav_bytes", len(wavBytes),
	)

	// 1) Lead lookup — phone match must be exact (same comparison the
	//    inbound router used to decide to send the call here). Receptionist
	//    routing only fires when the phone already has a call_transcripts
	//    row, so a missing lead here would mean the lead was deleted
	//    between dial and hang-up. Log and continue with leadID=0 so the
	//    transcript still lands (org-less rows are visible to Admins).
	var leadID, orgID int64
	if lead, err := rs.s.db.GetLeadByPhone(phone); err != nil {
		log.Warnw("receptionistSink: GetLeadByPhone failed", "err", err)
	} else if lead != nil {
		leadID = lead.ID
		orgID = lead.OrgID
	} else {
		log.Warnw("receptionistSink: no lead found for phone — saving transcript without lead_id")
	}

	// 2) WAV persistence. Filename mirrors the campaign-call recordings
	//    convention ({prefix}_{tsMs}.wav) so the dashboard's recording
	//    badge resolves automatically (`/api/recordings/{filename}`).
	recURL := ""
	if rs.s.cfg.RecordingsDir != "" && len(wavBytes) > 0 {
		filename := fmt.Sprintf("receptionist_%s_%d.wav", safeCallSid(callSid), time.Now().UnixMilli())
		fullPath := filepath.Join(rs.s.cfg.RecordingsDir, filename)
		if err := os.WriteFile(fullPath, wavBytes, 0o644); err != nil {
			log.Warnw("receptionistSink: WriteFile failed", "path", fullPath, "err", err)
		} else {
			recURL = "/api/recordings/" + filename
			log.Infow("receptionistSink: WAV saved", "path", fullPath, "bytes", len(wavBytes))
		}
	}

	// 3) call_transcripts insert. campaign_id=0 (NULL) flags this as a
	//    non-campaign call; the Past Conversations modal renders by
	//    lead_id and doesn't filter on campaign_id, so the row shows up.
	id, err := rs.s.db.SaveCallTranscript(leadID, 0, orgID, transcriptJSON, recURL, language, durationS)
	if err != nil {
		log.Warnw("receptionistSink: SaveCallTranscript failed", "err", err)
		return
	}
	log.Infow("receptionistSink: receptionist call persisted",
		"transcript_id", id,
		"lead_id", leadID,
		"org_id", orgID,
		"recording_url", recURL,
	)
}

// safeCallSid replaces characters that would be illegal in a filesystem
// path. Carrier call SIDs are normally hex-only but we're defensive in
// case a synthetic test SID slips through.
func safeCallSid(callSid string) string {
	if callSid == "" {
		return "unknown"
	}
	out := make([]rune, 0, len(callSid))
	for _, r := range callSid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}
