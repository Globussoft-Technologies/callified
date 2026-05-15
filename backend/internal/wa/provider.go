// Package wa handles WhatsApp multi-provider message parsing and sending.
package wa

import (
	"encoding/json"
	"strings"
)

// IncomingMessage is a normalised inbound WA message regardless of provider.
type IncomingMessage struct {
	ProviderMsgID string
	FromPhone     string
	ToPhone       string
	Text          string
	MediaURL      string
	MessageType   string // text, image, audio, document
	Provider      string
}

// ParseGupshup parses an inbound Gupshup webhook payload.
func ParseGupshup(body []byte) (*IncomingMessage, error) {
	var p struct {
		Type    string `json:"type"`
		Payload struct {
			ID      string `json:"id"`
			Source  string `json:"source"`
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
				Text string `json:"text"`
				URL  string `json:"url"`
			} `json:"payload"`
			Destination string `json:"destination"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &IncomingMessage{
		ProviderMsgID: p.Payload.ID,
		FromPhone:     p.Payload.Source,
		ToPhone:       p.Payload.Destination,
		Text:          p.Payload.Payload.Text,
		MediaURL:      p.Payload.Payload.URL,
		MessageType:   coalesce(p.Payload.Payload.Type, "text"),
		Provider:      "gupshup",
	}, nil
}

// ParseWati parses an inbound Wati webhook payload.
func ParseWati(body []byte) (*IncomingMessage, error) {
	var p struct {
		ID      string `json:"id"`
		WaID    string `json:"waId"`
		Type    string `json:"type"`
		Text    struct{ Body string } `json:"text"`
		Image   struct{ URL string }  `json:"image"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	text := p.Text.Body
	mediaURL := p.Image.URL
	msgType := p.Type
	if msgType == "" {
		msgType = "text"
	}
	return &IncomingMessage{
		ProviderMsgID: p.ID,
		FromPhone:     p.WaID,
		Text:          text,
		MediaURL:      mediaURL,
		MessageType:   msgType,
		Provider:      "wati",
	}, nil
}

// ParseInterakt parses an inbound Interakt webhook payload.
func ParseInterakt(body []byte) (*IncomingMessage, error) {
	var p struct {
		Data struct {
			Message struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				Message struct{ Text string } `json:"message"`
			} `json:"message"`
			Customer struct{ PhoneNumber string } `json:"customer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &IncomingMessage{
		ProviderMsgID: p.Data.Message.ID,
		FromPhone:     p.Data.Customer.PhoneNumber,
		Text:          p.Data.Message.Message.Text,
		MessageType:   coalesce(p.Data.Message.Type, "text"),
		Provider:      "interakt",
	}, nil
}

// ParseMeta parses an inbound Meta (WhatsApp Business API) webhook payload.
func ParseMeta(body []byte) (*IncomingMessage, error) {
	var p struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						ID   string `json:"id"`
						From string `json:"from"`
						Type string `json:"type"`
						Text struct{ Body string } `json:"text"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				return &IncomingMessage{
					ProviderMsgID: msg.ID,
					FromPhone:     msg.From,
					Text:          msg.Text.Body,
					MessageType:   coalesce(msg.Type, "text"),
					Provider:      "meta",
				}, nil
			}
		}
	}
	return nil, nil
}

// ParseAiSensei parses an inbound AiSensei webhook payload.
// AiSensei uses the same format as Gupshup.
func ParseAiSensei(body []byte) (*IncomingMessage, error) {
	msg, err := ParseGupshup(body)
	if err == nil && msg != nil {
		msg.Provider = "aisensei"
	}
	return msg, err
}

// ParseWaSender parses an inbound WaSender webhook payload.
//
// WaSender's payload shape (as of 2026-05): the message lives at
// data.messages.{key,message,messageType}, not data.{key,…}. For 1:1
// chats, remoteJid is now "<lid>@lid" instead of "<phone>@s.whatsapp.net"
// — the actual sender phone shows up in senderPn/cleanedSenderPn instead.
// Falling back to remoteJid when senderPn is missing keeps us compatible
// with the older payload shape (and group-chat events, which still use
// @s.whatsapp.net).
func ParseWaSender(body []byte) (*IncomingMessage, error) {
	type wsKey struct {
		RemoteJid       string `json:"remoteJid"`
		FromMe          bool   `json:"fromMe"`
		ID              string `json:"id"`
		SenderPn        string `json:"senderPn"`
		CleanedSenderPn string `json:"cleanedSenderPn"`
	}
	type wsMessage struct {
		Conversation        string `json:"conversation"`
		ExtendedTextMessage struct {
			Text string `json:"text"`
		} `json:"extendedTextMessage"`
	}
	var p struct {
		Event string `json:"event"`
		Data  struct {
			// New shape: { data: { messages: { key, message, messageType } } }
			Messages *struct {
				Key         wsKey     `json:"key"`
				Message     wsMessage `json:"message"`
				MessageType string    `json:"messageType"`
			} `json:"messages"`
			// Legacy shape: { data: { key, message, messageType } }
			Key         wsKey     `json:"key"`
			Message     wsMessage `json:"message"`
			MessageType string    `json:"messageType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	// Pick whichever shape matched.
	var key wsKey
	var msg wsMessage
	var msgType string
	if p.Data.Messages != nil {
		key = p.Data.Messages.Key
		msg = p.Data.Messages.Message
		msgType = p.Data.Messages.MessageType
	} else {
		key = p.Data.Key
		msg = p.Data.Message
		msgType = p.Data.MessageType
	}

	if key.FromMe {
		return nil, nil // skip echo of our own sent messages
	}
	text := msg.Conversation
	if text == "" {
		text = msg.ExtendedTextMessage.Text
	}
	// Prefer the cleaned-by-WaSender phone (already pure digits), then
	// senderPn (digits + @s.whatsapp.net), and only fall back to
	// remoteJid for older payloads or chats where the LID indirection
	// isn't applied. Strip both @s.whatsapp.net and @lid (modern 1:1)
	// and @c.us (legacy) suffixes — never strip leading digits.
	from := key.CleanedSenderPn
	if from == "" {
		from = key.SenderPn
	}
	if from == "" {
		from = key.RemoteJid
	}
	from = strings.TrimSuffix(from, "@s.whatsapp.net")
	from = strings.TrimSuffix(from, "@lid")
	from = strings.TrimSuffix(from, "@c.us")

	return &IncomingMessage{
		ProviderMsgID: key.ID,
		FromPhone:     from,
		Text:          text,
		MessageType:   coalesce(msgType, "text"),
		Provider:      "wasender",
	}, nil
}

// NormalizePhone ensures a phone number has a + prefix.
func NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return phone
	}
	if !strings.HasPrefix(phone, "+") {
		// Assume India if 10 digits
		if len(phone) == 10 {
			return "+91" + phone
		}
		return "+" + phone
	}
	return phone
}

func coalesce(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
