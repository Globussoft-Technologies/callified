package wshandler

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/globussoft/callified-backend/internal/llm"
)

var (
	clockTimePattern = regexp.MustCompile(`(?i)(?:\b\d{1,2}\s*:\s*\d{2}\b|\b\d{1,2}\s*(?:a\.?m\.?|p\.?m\.?)\b|\b(?:one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+o['’]?clock\b|\bat\s+(?:one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\b)`)
	// Calendar dates require date-shaped context so a bare time such as "12
	// PM" cannot also satisfy the date requirement. This covers the common
	// forms produced by STT and the model: "20th", "20 September", "September
	// 20", "20/09", and "the twentieth of this month".
	numericOrdinalDatePattern = regexp.MustCompile(`(?i)\b(?:[1-9]|[12]\d|3[01])(?:st|nd|rd|th)\b`)
	numericDatePattern        = regexp.MustCompile(`\b(?:[0-2]?\d|3[01])\s*[/.-]\s*(?:0?[1-9]|1[0-2])(?:\s*[/.-]\s*\d{2,4})?\b`)
	monthDatePattern          = regexp.MustCompile(`(?i)\b(?:(?:[1-9]|[12]\d|3[01])(?:st|nd|rd|th)?\s+(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)|(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(?:[1-9]|[12]\d|3[01])(?:st|nd|rd|th)?)\b`)
	wordOrdinalDatePattern    = regexp.MustCompile(`(?i)\b(?:the\s+)?(?:first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth|eleventh|twelfth|thirteenth|fourteenth|fifteenth|sixteenth|seventeenth|eighteenth|nineteenth|twentieth|twenty[ -](?:first|second|third|fourth|fifth|sixth|seventh|eighth|ninth)|thirtieth|thirty[ -]first)\s+(?:of\s+)?(?:this\s+month|next\s+month|[a-z]+day|jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t(?:ember)?)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\b`)
)

var appointmentDateMarkers = []string{
	// English and common Latin-script speech.
	"today", "tomorrow", "day after tomorrow", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "aaj", "kal",
	// Hindi and Marathi.
	"आज", "कल", "परसों", "उद्या", "परवा", "सोमवार", "मंगलवार", "मंगळवार", "बुधवार", "बुधवारी", "गुरुवार", "शुक्रवार", "शनिवार", "रविवार",
	// Telugu (native weekday names plus frequent English-name transliterations from STT).
	"ఈరోజు", "రేపు", "ఎల్లుండి", "సోమవారం", "మంగళవారం", "బుధవారం", "గురువారం", "శుక్రవారం", "శనివారం", "ఆదివారం",
	"మండే", "ట్యూస్డే", "ట్యూస్‌డే", "వెడ్నెస్డే", "థర్స్డే", "థర్స్‌డే", "ఫ్రైడే", "శాటర్డే", "సండే",
	// Tamil.
	"இன்று", "நாளை", "நாளை மறுநாள்", "திங்கள்", "செவ்வாய்", "புதன்", "வியாழன்", "வெள்ளி", "சனி", "ஞாயிறு",
	// Kannada.
	"ಇಂದು", "ನಾಳೆ", "ನಾಡಿದ್ದು", "ಸೋಮವಾರ", "ಮಂಗಳವಾರ", "ಬುಧವಾರ", "ಗುರುವಾರ", "ಶುಕ್ರವಾರ", "ಶನಿವಾರ", "ಭಾನುವಾರ",
	// Malayalam.
	"ഇന്ന്", "നാളെ", "മറ്റന്നാൾ", "തിങ്കളാഴ്ച", "ചൊവ്വാഴ്ച", "ബുധനാഴ്ച", "വ്യാഴാഴ്ച", "വെള്ളിയാഴ്ച", "ശനിയാഴ്ച", "ഞായറാഴ്ച",
	// Bengali.
	"আজ", "আগামীকাল", "পরশু", "সোমবার", "মঙ্গলবার", "বুধবার", "বৃহস্পতিবার", "শুক্রবার", "শনিবার", "রবিবার",
	// Gujarati.
	"આજે", "કાલે", "પરમદિવસે", "સોમવાર", "મંગળવાર", "બુધવાર", "ગુરુવાર", "શુક્રવાર", "શનિવાર", "રવિવાર",
	// Punjabi.
	"ਅੱਜ", "ਕੱਲ੍ਹ", "ਪਰਸੋਂ", "ਸੋਮਵਾਰ", "ਮੰਗਲਵਾਰ", "ਬੁੱਧਵਾਰ", "ਵੀਰਵਾਰ", "ਸ਼ੁੱਕਰਵਾਰ", "ਸ਼ੁੱਕਰਵਾਰ", "ਸ਼ਨੀਵਾਰ", "ਐਤਵਾਰ",
}

var appointmentTimeMarkers = []string{
	"am", "pm", "a.m", "p.m", "o'clock", "noon", "midnight", "morning", "afternoon", "evening",
	"बजे", "सुबह", "दोपहर", "शाम", "रात", "एएम", "पीएम", "वाजता", "सकाळी", "दुपारी", "संध्याकाळी", "रात्री",
	"గంట", "గంటకు", "గంటకి", "గంటలకు", "గంటలకి", "ఉదయం", "మధ్యాహ్నం", "సాయంత్రం", "రాత్రి", "ఏఎం", "ఎఎం", "పీఎం",
	"மணி", "காலை", "மதியம்", "மாலை", "இரவு", "ஏஎம்", "பிஎம்",
	"ಗಂಟೆ", "ಗಂಟೆಗೆ", "ಬೆಳಿಗ್ಗೆ", "ಮಧ್ಯಾಹ್ನ", "ಸಂಜೆ", "ರಾತ್ರಿ", "ಎಎಂ", "ಪಿಎಂ",
	"മണി", "മണിക്ക്", "രാവിലെ", "ഉച്ചയ്ക്ക്", "വൈകുന്നേരം", "രാത്രി", "എഎം", "പിഎം",
	"টা", "টায়", "বাজে", "সকাল", "দুপুর", "বিকেল", "সন্ধ্যা", "রাত", "এএম", "পিএম",
	"વાગ્યે", "સવારે", "બપોરે", "સાંજે", "રાત્રે", "એએમ", "પીએમ",
	"ਵਜੇ", "ਸਵੇਰੇ", "ਦੁਪਹਿਰ", "ਸ਼ਾਮ", "ਰਾਤ", "ਏਐਮ", "ਪੀਐਮ",
}

var appointmentNumberWords = []string{
	"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "twelv",
	"एक", "दो", "तीन", "चार", "पांच", "पाँच", "छह", "सात", "आठ", "नौ", "दस", "ग्यारह", "बारह", "बारा",
	"ఒంటి", "ఒకటి", "రెండు", "మూడు", "నాలుగు", "ఐదు", "ఆరు", "ఏడు", "ఎనిమిది", "తొమ్మిది", "పది", "పదకొండు", "పన్నెండు", "ట్వెల్వ్",
	"ஒன்று", "இரண்டு", "மூன்று", "நான்கு", "ஐந்து", "ஆறு", "ஏழு", "எட்டு", "ஒன்பது", "பத்து", "பதினொன்று", "பன்னிரண்டு",
	"ಒಂದು", "ಎರಡು", "ಮೂರು", "ನಾಲ್ಕು", "ಐದು", "ಆರು", "ಏಳು", "ಎಂಟು", "ಒಂಬತ್ತು", "ಹತ್ತು", "ಹನ್ನೊಂದು", "ಹನ್ನೆರಡು",
	"ഒന്ന്", "രണ്ട്", "മൂന്ന്", "നാല്", "അഞ്ച്", "ആറ്", "ഏഴ്", "എട്ട്", "ഒമ്പത്", "പത്ത്", "പതിനൊന്ന്", "പന്ത്രണ്ട്",
	"একটা", "দুই", "তিন", "চার", "পাঁচ", "ছয়", "সাত", "আট", "নয়", "দশ", "এগারো", "বারো",
	"એક", "બે", "ત્રણ", "ચાર", "પાંચ", "છ", "સાત", "આઠ", "નવ", "દસ", "અગિયાર", "બાર",
	"ਇੱਕ", "ਦੋ", "ਤਿੰਨ", "ਚਾਰ", "ਪੰਜ", "ਛੇ", "ਸੱਤ", "ਅੱਠ", "ਨੌਂ", "ਦਸ", "ਗਿਆਰਾਂ", "ਬਾਰਾਂ",
}

// isPathologicalTranscript rejects ASR degeneration such as a single word
// repeated hundreds of times. These results are not customer speech and must
// never enter conversation history or influence a terminal action.
func isPathologicalTranscript(text string) bool {
	norm := normalizeQuestionText(text)
	if norm == "" {
		return false
	}
	if len([]rune(norm)) > 2000 {
		return true
	}
	words := strings.Fields(norm)
	if len(words) < 12 {
		return false
	}

	counts := make(map[string]int, len(words))
	maxCount, run, maxRun := 0, 0, 0
	previous := ""
	for _, word := range words {
		counts[word]++
		if counts[word] > maxCount {
			maxCount = counts[word]
		}
		if word == previous {
			run++
		} else {
			previous = word
			run = 1
		}
		if run > maxRun {
			maxRun = run
		}
	}
	return maxRun >= 8 || (len(words) >= 20 && maxCount*100/len(words) >= 70)
}

func isRepeatOrClarificationRequest(text string) bool {
	norm := normalizeQuestionText(text)
	if norm == "" {
		return false
	}
	markers := []string{
		"repeat", "say that again", "say it again", "come again", "didnt get", "did not get",
		"dont understand", "do not understand", "couldnt hear", "could not hear", "not audible",
		"phir se", "dobara", "samajh nahi", "sunai nahi", "फिर से", "दोबारा", "समझ नहीं", "सुनाई नहीं",
		"malli", "marokasari", "ardham kaledu", "vinipinchaledu", "మళ్లీ", "మరొకసారి", "అర్థం కాలేదు", "వినిపించలేదు",
		"meendum", "puriyala", "kekkala", "மீண்டும்", "புரியவில்லை", "கேட்கவில்லை",
		"ಮತ್ತೆ", "ಅರ್ಥವಾಗಲಿಲ್ಲ", "ಕೇಳಿಸಲಿಲ್ಲ", "വീണ്ടും", "മനസ്സിലായില്ല", "കേട്ടില്ല",
		"पुन्हा", "समजले नाही", "ऐकू आले नाही", "আবার বলুন", "বুঝিনি", "শুনতে পাইনি",
		"ફરીથી", "સમજાયું નહીં", "સંભળાયું નહીં", "ਦੁਬਾਰਾ", "ਫਿਰ ਦੱਸੋ", "ਸਮਝ ਨਹੀਂ", "ਸੁਣਾਈ ਨਹੀਂ",
	}
	for _, marker := range markers {
		if strings.Contains(norm, marker) {
			return true
		}
	}
	return false
}

// isMeaningfulBargeInPartial avoids cutting TTS for a one-syllable or corrupt
// partial. Normal multi-word interruptions still stop playback promptly.
func isMeaningfulBargeInPartial(text string) bool {
	if isPathologicalTranscript(text) {
		return false
	}
	norm := normalizeQuestionText(text)
	if len([]rune(norm)) < 2 || isKnownFiller(norm) {
		return false
	}
	words := strings.Fields(norm)
	if len(words) >= 3 {
		allSame := true
		for _, word := range words[1:] {
			if word != words[0] {
				allSame = false
				break
			}
		}
		if allSame {
			return false
		}
	}
	if len(words) > 1 {
		return true
	}
	switch norm {
	case "yes", "no", "hello", "stop", "wait", "repeat", "haan", "nahi", "avunu", "ledu", "ஆமாம்", "இல்லை", "ಹೌದು", "ಇಲ್ಲ":
		return true
	default:
		return false
	}
}

func terminalActionAllowed(outcome, transcript string, history []llm.ChatMessage) bool {
	if isPathologicalTranscript(transcript) || isRepeatOrClarificationRequest(transcript) {
		return false
	}
	switch outcome {
	case "appointment_booked":
		return appointmentHasDayAndTime(transcript, history)
	case "customer_declined":
		switch normalizeQuestionText(transcript) {
		case "no", "nope", "nahi", "nahin", "ledu", "వద్దు", "नहीं", "இல்லை", "ಬೇಡ", "വേണ്ട", "না", "ના", "ਨਹੀਂ":
			return true
		}
		return containsAnyNormalized(transcript, []string{
			"not interested", "no thanks", "dont need", "do not need", "ill pass", "i will pass",
			"interest ledu", "వద్దు", "ఆసక్తి లేదు", "नहीं चाहिए", "रुचि नहीं", "வேண்டாம்", "விருப்பமில்லை",
		})
	case "customer_requested_end":
		return containsAnyNormalized(transcript, []string{
			"hang up", "end the call", "stop the call", "dont call", "do not call", "goodbye", "bye",
			"call cut", "phone pettu", "కాల్ కట్", "போனை வை", "फोन रखो", "ಕಾಲ್ ಕಟ್",
		})
	default:
		// Textual [HANGUP] remains supported for non-Gemini providers, but a
		// repeat/clarification turn above can never close the call.
		return true
	}
}

func appointmentHasDayAndTime(transcript string, history []llm.ChatMessage) bool {
	// Keep the active scheduling exchange available through several clarification
	// turns. Four messages was too short: after two retries the agreed weekday
	// disappeared and every subsequent valid time was rejected.
	parts := make([]string, 0, 17)
	start := len(history) - 16
	if start < 0 {
		start = 0
	}
	for _, message := range history[start:] {
		parts = append(parts, message.Text)
	}
	parts = append(parts, transcript)
	context := strings.ToLower(strings.Join(parts, " "))

	hasDate := containsAnyLiteral(context, appointmentDateMarkers)
	if !hasDate {
		hasDate = numericOrdinalDatePattern.MatchString(context) ||
			numericDatePattern.MatchString(context) ||
			monthDatePattern.MatchString(context) ||
			wordOrdinalDatePattern.MatchString(context)
	}
	if !hasDate {
		return false
	}
	if clockTimePattern.MatchString(context) {
		return true
	}
	return containsAnyLiteral(context, appointmentTimeMarkers) && containsDigitOrNumberWord(context)
}

func containsDigitOrNumberWord(text string) bool {
	for _, r := range text {
		if unicode.IsDigit(r) {
			return true
		}
	}
	for _, word := range appointmentNumberWords {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func containsAnyLiteral(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containsAnyNormalized(text string, markers []string) bool {
	norm := normalizeQuestionText(text)
	for _, marker := range markers {
		if strings.Contains(norm, normalizeQuestionText(marker)) {
			return true
		}
	}
	return false
}

func terminalRecoveryLine(language, outcome string) string {
	if outcome == "appointment_booked" {
		switch language {
		case "hi":
			return "कृपया डेमो के लिए सही समय भी बताइए।"
		case "mr":
			return "कृपया डेमोसाठी नेमकी वेळही सांगा।"
		case "bn":
			return "ডেমোর সঠিক সময়টিও বলুন।"
		case "gu":
			return "કૃપા કરીને ડેમો માટે ચોક્કસ સમય પણ જણાવો।"
		case "pa":
			return "ਕਿਰਪਾ ਕਰਕੇ ਡੈਮੋ ਦਾ ਸਹੀ ਸਮਾਂ ਵੀ ਦੱਸੋ।"
		case "te":
			return "డెమో కోసం కచ్చితమైన సమయం కూడా చెప్పండి।"
		case "ta":
			return "டெமோவிற்கான சரியான நேரத்தையும் சொல்லுங்கள்।"
		case "kn":
			return "ಡೆಮೊಗೆ ನಿಖರವಾದ ಸಮಯವನ್ನೂ ತಿಳಿಸಿ।"
		case "ml":
			return "ഡെമോയ്ക്കുള്ള കൃത്യമായ സമയവും പറയാമോ।"
		default:
			return "What exact time works for the demo?"
		}
	}
	return "Sorry, could you please say that again?"
}
