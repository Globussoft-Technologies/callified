package realtime

import (
	"encoding/binary"
	"fmt"
	"mime"
	"strconv"
	"strings"
)

// ResolveVoice accepts the Gemini voices offered by the app. Unknown gateway
// voices can still be explicitly configured through GEMINI_LIVE_VOICE.
func ResolveVoice(campaign, configured string) string {
	campaign = strings.TrimSpace(campaign)
	if strings.Contains(" Kore Puck Charon Fenrir Aoede Leda Orus Zephyr Callirrhoe Autonoe Enceladus Iapetus Umbriel Algieba Despina Erinome Algenib Rasalgethi Laomedeia Achernar Alnilam Schedar Gacrux Pulcherrima Achird Zubenelgenubi Vindemiatrix Sadachbia Sadaltager Sulafat en-in-csagent-12 en-in-csagent-1 en-in-csagent-5 ", " "+campaign+" ") && campaign != "" {
		return campaign
	}
	return strings.TrimSpace(configured)
}

func livePCMRate(value string) (int, error) {
	// Some existing gateways omit metadata; the Live default is 24 kHz PCM.
	if value == "" {
		return 24000, nil
	}
	media, params, err := mime.ParseMediaType(value)
	if err != nil || media != "audio/pcm" {
		return 0, fmt.Errorf("gemini live: unsupported audio format %q", value)
	}
	if channels := params["channels"]; channels != "" && channels != "1" {
		return 0, fmt.Errorf("gemini live: expected mono audio")
	}
	rate := 24000
	if text := params["rate"]; text != "" {
		rate, err = strconv.Atoi(text)
	}
	if err != nil || rate < 8000 || rate > 96000 {
		return 0, fmt.Errorf("gemini live: invalid PCM sample rate %q", params["rate"])
	}
	return rate, nil
}

// livePCMNormalizer converts little-endian mono PCM16 to 24 kHz, preserving
// fractional position and the boundary sample across network chunks.
type livePCMNormalizer struct {
	rate     int
	samples  []int16
	position int64 // source position in units of 1/24000 sample
}

func (r *livePCMNormalizer) process(pcm []byte, rate int) ([]byte, error) {
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("gemini live: incomplete PCM16 sample")
	}
	if r.rate != rate {
		*r = livePCMNormalizer{rate: rate}
	}
	if rate == 24000 {
		return pcm, nil
	}
	for i := 0; i < len(pcm); i += 2 {
		r.samples = append(r.samples, int16(binary.LittleEndian.Uint16(pcm[i:])))
	}
	out := make([]byte, 0, len(pcm)*24000/rate+2)
	for r.position/24000+1 < int64(len(r.samples)) {
		index, fraction := r.position/24000, r.position%24000
		a, b := int64(r.samples[index]), int64(r.samples[index+1])
		sample := int16(a + (b-a)*fraction/24000)
		out = binary.LittleEndian.AppendUint16(out, uint16(sample))
		r.position += int64(rate)
	}
	drop := int(r.position / 24000)
	if drop >= len(r.samples) {
		drop = len(r.samples) - 1
	}
	if drop > 0 {
		r.samples = append(r.samples[:0], r.samples[drop:]...)
		r.position -= int64(drop) * 24000
	}
	return out, nil
}
