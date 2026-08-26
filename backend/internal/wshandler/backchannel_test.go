package wshandler

import "testing"

func TestIsExplicitLangSwitch(t *testing.T) {
	cases := []struct {
		text       string
		wantLang   string
		wantSwitch bool
	}{
		// English requests
		{"can you speak in kannada", "kn", true},
		{"switch to hindi", "hi", true},
		{"speak in tamil", "ta", true},

		// Romanized/transliterated requests
		{"kannada alli", "kn", true},
		{"kannada dhalli mathadi", "kn", true},
		{"hindi mein bolo", "hi", true},
		{"marathi madhe", "mr", true},
		{"punjabi vich bolo", "pa", true},
		{"panjabi me bolo", "pa", true},
		{"telugu lo matladandi", "te", true},
		{"tamil la pesunga", "ta", true},
		{"malayalam parayu", "ml", true},
		{"bengali te bolo", "bn", true},
		{"gujarati ma", "gu", true},

		// Native script requests
		{"ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡ್ತೀರಾ?", "kn", true},
		{"ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡಿ", "kn", true},
		{"ಹಲೋ, ಕನ್ನಡದಲ್ಲಿ ಮಾತಾಡಿ", "kn", true},
		{"हिंदी में बोलो", "hi", true},
		{"मराठीत बोला", "mr", true},
		{"తెలుగులో మాట్లాడండి", "te", true},
		{"தமிழில் பேசுங்கள்", "ta", true},
		{"বাংলায় বলো", "bn", true},
		{"ગુજરાતીમાં બોલો", "gu", true},
		{"ਪੰਜਾਬੀ ਵਿੱਚ ਬੋਲੋ", "pa", true},
		{"മലയാളത്തിൽ പറയു", "ml", true},

		// Ambiguous questions - in a voice call context these are treated as
		// requests to switch, since the customer is asking about the agent's
		// ability to speak a language.
		{"do you speak hindi?", "hi", true},
		{"can you speak in kannada?", "kn", true},

		// Non-switch phrases
		{"i like kannada food", "", false},
	}

	for _, c := range cases {
		gotLang, gotSwitch := isExplicitLangSwitch(c.text)
		if gotSwitch != c.wantSwitch || gotLang != c.wantLang {
			t.Errorf("isExplicitLangSwitch(%q) = (%q, %v), want (%q, %v)",
				c.text, gotLang, gotSwitch, c.wantLang, c.wantSwitch)
		}
	}
}
