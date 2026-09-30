package wshandler

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type appointmentSlotValidation string

const (
	appointmentSlotValid       appointmentSlotValidation = "valid"
	appointmentSlotMissingDate appointmentSlotValidation = "missing_date"
	appointmentSlotMissingTime appointmentSlotValidation = "missing_time"
	appointmentSlotInvalid     appointmentSlotValidation = "invalid"
	appointmentSlotPast        appointmentSlotValidation = "past"
)

var appointmentClockPattern = regexp.MustCompile(`(?i)^\s*(\d{1,2})(?:\s*:\s*(\d{1,2}))?\s*(a\.?m\.?|p\.?m\.?)?\s*$`)

// validateAppointmentSlot is the only deterministic Gemini Live appointment
// gate. Conversation and confirmation order belong to the configured call
// flow; Callified only verifies that the tool supplied a real future instant.
// A slot later today is valid, as is any later calendar date.
func validateAppointmentSlot(dateText, timeText, timezone string, now time.Time) appointmentSlotValidation {
	dateText = strings.TrimSpace(dateText)
	timeText = strings.TrimSpace(timeText)
	if dateText == "" {
		return appointmentSlotMissingDate
	}
	if timeText == "" {
		return appointmentSlotMissingTime
	}

	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		location, _ = time.LoadLocation("Asia/Kolkata")
	}
	now = now.In(location)
	date, ok := parseAppointmentDate(dateText, now, location)
	if !ok {
		return appointmentSlotInvalid
	}
	hour, minute, ok := parseAppointmentTime(timeText)
	if !ok {
		return appointmentSlotInvalid
	}
	slot := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
	if !slot.After(now) {
		return appointmentSlotPast
	}
	return appointmentSlotValid
}

func parseAppointmentDate(text string, now time.Time, location *time.Location) (time.Time, bool) {
	norm := strings.ToLower(strings.TrimSpace(text))
	norm = strings.Join(strings.Fields(strings.NewReplacer(
		",", " ", ".", " ", "the ", "", " of ", " ",
	).Replace(norm)), " ")

	switch norm {
	case "today", "aaj", "आज", "ఈరోజు", "இன்று", "ಇಂದು", "ഇന്ന്", "আজ", "આજે", "ਅੱਜ":
		return startOfAppointmentDay(now, location), true
	case "tomorrow", "kal", "कल", "उद्या", "రేపు", "நாளை", "ನಾಳೆ", "നാളെ", "আগামীকাল", "કાલે", "ਕੱਲ੍ਹ":
		return startOfAppointmentDay(now.AddDate(0, 0, 1), location), true
	case "day after tomorrow", "parso", "परसों", "परवा", "ఎల్లుండి", "நாளை மறுநாள்", "ನಾಡಿದ್ದು", "മറ്റന്നാൾ", "পরশু", "પરમદિવસે", "ਪਰਸੋਂ":
		return startOfAppointmentDay(now.AddDate(0, 0, 2), location), true
	}

	if weekday, ok := appointmentWeekday(norm); ok {
		days := (int(weekday) - int(now.Weekday()) + 7) % 7
		return startOfAppointmentDay(now.AddDate(0, 0, days), location), true
	}

	// Prefer an unambiguous ISO date. Other layouts remain accepted for
	// compatibility with existing Gemini tool calls.
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2/1/2006", "02/01/2006", "2-1-2006", "02-01-2006"} {
		if parsed, err := time.ParseInLocation(layout, norm, location); err == nil {
			return startOfAppointmentDay(parsed, location), true
		}
	}

	norm = normalizeAppointmentOrdinal(norm)
	for _, layout := range []string{"2 January 2006", "January 2 2006", "2 Jan 2006", "Jan 2 2006"} {
		if parsed, err := time.ParseInLocation(layout, norm, location); err == nil {
			return startOfAppointmentDay(parsed, location), true
		}
	}
	for _, layout := range []string{"2 January", "January 2", "2 Jan", "Jan 2"} {
		if parsed, err := time.ParseInLocation(layout, norm, location); err == nil {
			parsed = time.Date(now.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, location)
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseAppointmentTime(text string) (int, int, bool) {
	norm := strings.ToLower(strings.TrimSpace(text))
	switch norm {
	case "noon":
		return 12, 0, true
	case "midnight":
		return 0, 0, true
	}
	for word, number := range appointmentHourWords {
		norm = regexp.MustCompile(`\b`+word+`\b`).ReplaceAllString(norm, strconv.Itoa(number))
	}
	match := appointmentClockPattern.FindStringSubmatch(norm)
	if match == nil {
		return 0, 0, false
	}
	hour, _ := strconv.Atoi(match[1])
	minute := 0
	if match[2] != "" {
		minute, _ = strconv.Atoi(match[2])
	}
	if minute > 59 {
		return 0, 0, false
	}
	meridiem := strings.NewReplacer(".", "", " ", "").Replace(match[3])
	if meridiem != "" {
		if hour < 1 || hour > 12 {
			return 0, 0, false
		}
		if meridiem == "pm" && hour != 12 {
			hour += 12
		}
		if meridiem == "am" && hour == 12 {
			hour = 0
		}
	} else if hour > 23 || (match[2] == "" && hour <= 12) {
		// A bare "three" or "3" is not an exact clock time because AM/PM is
		// unknown. Twenty-four-hour values and HH:MM remain unambiguous.
		return 0, 0, false
	}
	return hour, minute, true
}

func startOfAppointmentDay(value time.Time, location *time.Location) time.Time {
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

func appointmentWeekday(text string) (time.Weekday, bool) {
	weekdays := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday,
		"saturday": time.Saturday,
	}
	weekday, ok := weekdays[text]
	return weekday, ok
}

var appointmentHourWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
}

func normalizeAppointmentOrdinal(text string) string {
	replacer := strings.NewReplacer(
		"first", "1", "second", "2", "third", "3", "fourth", "4", "fifth", "5",
		"sixth", "6", "seventh", "7", "eighth", "8", "ninth", "9", "tenth", "10",
		"eleventh", "11", "twelfth", "12", "thirteenth", "13", "fourteenth", "14",
		"fifteenth", "15", "sixteenth", "16", "seventeenth", "17", "eighteenth", "18",
		"nineteenth", "19", "twentieth", "20", "twenty first", "21", "twenty-first", "21",
		"twenty second", "22", "twenty-second", "22", "twenty third", "23", "twenty-third", "23",
		"twenty fourth", "24", "twenty-fourth", "24", "twenty fifth", "25", "twenty-fifth", "25",
		"twenty sixth", "26", "twenty-sixth", "26", "twenty seventh", "27", "twenty-seventh", "27",
		"twenty eighth", "28", "twenty-eighth", "28", "twenty ninth", "29", "twenty-ninth", "29",
		"thirtieth", "30", "thirty first", "31", "thirty-first", "31",
	)
	text = replacer.Replace(text)
	return regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)\b`).ReplaceAllString(text, "$1")
}
