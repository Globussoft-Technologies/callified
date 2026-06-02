// Package wa handles WhatsApp multi-provider message parsing and sending.
package wa

import (
	"encoding/json"
	"fmt"
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
// It also returns any delivery-failure statuses via the second return value
// so callers can log them. Normal (non-error) statuses are ignored.
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
					Statuses []struct {
						ID       string `json:"id"`
						Status   string `json:"status"`
						RecipientID string `json:"recipient_id"`
						Errors   []struct {
							Code    int    `json:"code"`
							Title   string `json:"title"`
							Message string `json:"message"`
						} `json:"errors"`
					} `json:"statuses"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	for _, entry := range p.Entry {
		for _, change := range entry.Changes {
			// Log any failed delivery statuses
			for _, st := range change.Value.Statuses {
				if st.Status == "failed" && len(st.Errors) > 0 {
					return nil, fmt.Errorf("meta delivery failed for %s (wamid:%s): code=%d %s — %s",
						st.RecipientID, st.ID, st.Errors[0].Code, st.Errors[0].Title, st.Errors[0].Message)
				}
			}
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
// WaSender wraps the message under data.messages (not data directly):
//
//	{"event":"messages.upsert","data":{"messages":{"key":{...},"message":{...},"messageBody":"..."}}}
//
// Only processes messages.upsert / messages.received events with fromMe=false
// so that echoes of our own sent messages and status-ack events are dropped
// before they create spurious conversation rows in the DB.
func ParseWaSender(body []byte) (*IncomingMessage, error) {
	var p struct {
		Event string `json:"event"`
		Data  struct {
			Messages struct {
				Key struct {
					RemoteJid      string `json:"remoteJid"`
					FromMe         bool   `json:"fromMe"`
					ID             string `json:"id"`
					SenderPn       string `json:"senderPn"`       // e.g. "919019781278@s.whatsapp.net"
					CleanedSenderPn string `json:"cleanedSenderPn"` // e.g. "919019781278"
				} `json:"key"`
				Message struct {
					Conversation        string `json:"conversation"`
					ExtendedTextMessage struct {
						Text string `json:"text"`
					} `json:"extendedTextMessage"`
				} `json:"message"`
				MessageBody string `json:"messageBody"` // flat copy WaSender also sends
				MessageType string `json:"messageType"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	// Only handle inbound message events. WaSender also delivers
	// connection.update, qr.update, messages.update (status acks for
	// delivered/read), and presence.update — none of those carry a
	// customer message. Both messages.upsert and messages.received
	// can carry an actual inbound text depending on WaSender version.
	if p.Event != "messages.upsert" && p.Event != "messages.received" {
		return nil, nil
	}
	if p.Data.Messages.Key.FromMe {
		return nil, nil // skip echo of our own sent messages
	}
	// Prefer the nested message body, fall back to the flat messageBody field.
	text := p.Data.Messages.Message.Conversation
	if text == "" {
		text = p.Data.Messages.Message.ExtendedTextMessage.Text
	}
	if text == "" {
		text = p.Data.Messages.MessageBody
	}
	// Derive the from-phone. WaSender uses two JID formats:
	//   - "917795740488@s.whatsapp.net" → strip suffix → phone
	//   - "157737388404946@lid"         → LID (no phone) → use cleanedSenderPn
	// cleanedSenderPn is the actual phone without domain, available on all
	// inbound messages regardless of addressingMode.
	from := p.Data.Messages.Key.CleanedSenderPn
	if from == "" {
		from = strings.TrimSuffix(p.Data.Messages.Key.RemoteJid, "@s.whatsapp.net")
		from = strings.TrimSuffix(from, "@lid") // drop LID suffix if cleanedSenderPn absent
	}
	return &IncomingMessage{
		ProviderMsgID: p.Data.Messages.Key.ID,
		FromPhone:     from,
		Text:          text,
		MessageType:   coalesce(p.Data.Messages.MessageType, "text"),
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
