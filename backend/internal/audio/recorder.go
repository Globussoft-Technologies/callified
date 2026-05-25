package audio

import (
	"bytes"
	"encoding/binary"
	"time"
)

const (
	recSampleRate  = 8000
	recBitDepth    = 16
	recChannels    = 2 // stereo: L=user, R=AI
	recBytesPerSec = recSampleRate * recBitDepth / 8 // per mono channel
)

// TimedChunk is a PCM 16-bit 8kHz mono audio chunk with the wall-clock
// timestamp at which it was captured. Used for stereo WAV assembly.
type TimedChunk struct {
	Ts   time.Time
	Data []byte // raw PCM 16-bit little-endian at 8kHz
}

// micTargetPeak is the target peak amplitude for the mic channel after gain.
// 24000 ≈ -2.7 dBFS — loud enough to hear clearly, safe from clipping.
const micTargetPeak = 24000

// BuildStereoWAV assembles mic (L) and TTS (R) chunks into a stereo WAV file.
// Returns nil if both slices are empty.
func BuildStereoWAV(micChunks, ttsChunks []TimedChunk) []byte {
	if len(micChunks) == 0 && len(ttsChunks) == 0 {
		return nil
	}
	tStart, tEnd := timeBounds(micChunks, ttsChunks)
	totalDur := tEnd.Sub(tStart) + 500*time.Millisecond
	totalSamples := int(totalDur.Seconds() * float64(recSampleRate))
	if totalSamples <= 0 {
		return nil
	}

	userBuf := make([]byte, totalSamples*2)
	aiBuf := make([]byte, totalSamples*2)
	fillBuf(userBuf, micChunks, tStart)
	fillBuf(aiBuf, ttsChunks, tStart)

	// Normalize mic channel: find peak and scale up to micTargetPeak.
	// Only boost — never attenuate (if caller is already loud, leave as-is).
	normalizeMic(userBuf)

	// Interleave: [L0 L0 R0 R0 | L1 L1 R1 R1 | ...]
	stereo := make([]byte, totalSamples*4)
	for i := range totalSamples {
		copy(stereo[i*4:i*4+2], userBuf[i*2:i*2+2])
		copy(stereo[i*4+2:i*4+4], aiBuf[i*2:i*2+2])
	}
	return encodeWAV(stereo, recChannels, recSampleRate, recBitDepth)
}

// normalizeMic scales the mic PCM buffer so the peak reaches micTargetPeak.
// Only amplifies — if the peak is already at or above the target, no change.
func normalizeMic(buf []byte) {
	if len(buf) < 2 {
		return
	}
	// Find peak
	var peak int16
	for i := 0; i+1 < len(buf); i += 2 {
		s := int16(uint16(buf[i]) | uint16(buf[i+1])<<8)
		if s < 0 {
			s = -s
		}
		if s > peak {
			peak = s
		}
	}
	if peak == 0 || int(peak) >= micTargetPeak {
		return // silent or already loud enough
	}
	gain := float32(micTargetPeak) / float32(peak)
	for i := 0; i+1 < len(buf); i += 2 {
		s := int16(uint16(buf[i]) | uint16(buf[i+1])<<8)
		scaled := int32(float32(s) * gain)
		if scaled > 32767 {
			scaled = 32767
		} else if scaled < -32768 {
			scaled = -32768
		}
		v := uint16(int16(scaled))
		buf[i] = byte(v)
		buf[i+1] = byte(v >> 8)
	}
}

func timeBounds(a, b []TimedChunk) (tMin, tMax time.Time) {
	all := append(append([]TimedChunk(nil), a...), b...)
	if len(all) == 0 {
		return
	}
	tMin, tMax = all[0].Ts, all[0].Ts
	for _, c := range all {
		end := c.Ts.Add(time.Duration(len(c.Data)/2) * time.Second / recSampleRate)
		if c.Ts.Before(tMin) {
			tMin = c.Ts
		}
		if end.After(tMax) {
			tMax = end
		}
	}
	return
}

func fillBuf(buf []byte, chunks []TimedChunk, tStart time.Time) {
	for _, c := range chunks {
		offsetBytes := int(c.Ts.Sub(tStart).Seconds()*float64(recSampleRate)) * 2
		end := offsetBytes + len(c.Data)
		if offsetBytes >= len(buf) || end <= 0 {
			continue
		}
		if end > len(buf) {
			end = len(buf)
		}
		src := 0
		if offsetBytes < 0 {
			src = -offsetBytes
			offsetBytes = 0
		}
		copy(buf[offsetBytes:end], c.Data[src:end-offsetBytes+src])
	}
}

func encodeWAV(data []byte, channels, sampleRate, bitDepth int) []byte {
	var buf bytes.Buffer
	dataLen := len(data)
	blockAlign := channels * bitDepth / 8
	byteRate := sampleRate * blockAlign

	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen)) //nolint:errcheck
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))              //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint16(1))               // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(channels))        //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))      //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))        //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))      //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint16(bitDepth))        //nolint:errcheck
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataLen)) //nolint:errcheck
	buf.Write(data)
	return buf.Bytes()
}
