package wshandler

import "strings"

// resumeNormalClose requires meaningful recognized customer speech. Audio
// activity alone may be echo/noise and must not reopen a completed call.
func (s *CallSession) ResumeNormalClose(text string) bool {
	if !s.finalCloseReq.Load() || !shouldResumeClosing(text) {
		return false
	}
	s.closeMu.Lock()
	if s.closeCommitted || s.IsMaxDurationClosing() || !s.finalCloseReq.Load() {
		s.closeMu.Unlock()
		return false
	}
	s.finalCloseReq.Store(false)
	s.finalCloseAudio.Store(false)
	s.hangupReq.Store(false)
	s.bargeInPending.Store(false)
	s.interruptActiveTTS()
	s.PlaybackTracker.Reset()
	s.closeMu.Unlock()
	s.Log.Info("normal closing cancelled by customer continuation")
	return true
}

func shouldResumeClosing(text string) bool {
	if isPathologicalTranscript(text) || isKnownFiller(text) {
		return false
	}
	if isRepeatOrClarificationRequest(text) {
		return true
	}
	markers := []string{
		"wait", "hold on", "one more thing", "one more question", "another question", "actually",
		"what", "how", "where", "when", "why", "which", "can you", "could you", "tell me", "explain",
		"is there", "is it", "does it", "do you", "will it", "would you", "are you", "i have a question",
		"रुको", "रुकिए", "एक बात", "क्या", "कैसे", "कहाँ", "कब", "क्यों",
		"ఆగండి", "ఇంకో ప్రశ్న", "ఏమిటి", "ఏంటి", "ఎలా", "ఎక్కడ", "ఎప్పుడు", "ఎందుకు",
		"aagandi", "enti", "ela", "ekkada", "ఎవరు", "ఎవరి",
		"ನಿಲ್ಲಿ", "ಏನು", "ಹೇಗೆ", "ಎಲ್ಲಿ", "ಯಾವಾಗ", "ಏಕೆ",
		"நிறுத்துங்கள்", "என்ன", "எப்படி", "எங்கே", "எப்போது",
		"থামুন", "কী", "কিভাবে", "কোথায়", "কখন",
		"थांबा", "काय", "कसे", "कुठे", "थांबवा",
		"રોકો", "શું", "કેવી રીતે", "ક્યાં", "ਕਿਵੇਂ", "ਕਿੱਥੇ", "ਰੁਕੋ",
		"നിൽക്കൂ", "എന്ത്", "എങ്ങനെ", "എവിടെ",
	}
	norm := " " + normalizeQuestionText(text) + " "
	for _, marker := range markers {
		if strings.Contains(norm, " "+normalizeQuestionText(marker)+" ") {
			return true
		}
	}
	return false
}

// CommitHangup serializes the final irreversible hangup with cancellation.
// A stale sentinel from an interrupted goodbye must never end the resumed call.
func (s *CallSession) CommitHangup(epoch uint64) bool {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closeCommitted || (!s.IsMaxDurationClosing() && (epoch != s.PlaybackEpoch() || !s.HangupRequested())) {
		return false
	}
	s.closeCommitted = true
	return true
}
