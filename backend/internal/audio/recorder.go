package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"
)

const (
	recSampleRate  = 8000
	recBitDepth    = 16
	recChannels    = 2                               // stereo: L=user, R=AI
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

	// Match mic level to TTS level so both channels sound equally loud in the
	// stored recording. Use the TTS peak as the target; fall back to
	// micTargetPeak when TTS is silent or very quiet.
	target := pcmPeak(aiBuf)
	if target < micTargetPeak {
		target = micTargetPeak
	}
	normalizeTo(userBuf, target)

	// Interleave: [L0 L0 R0 R0 | L1 L1 R1 R1 | ...]
	stereo := make([]byte, totalSamples*4)
	for i := range totalSamples {
		copy(stereo[i*4:i*4+2], userBuf[i*2:i*2+2])
		copy(stereo[i*4+2:i*4+4], aiBuf[i*2:i*2+2])
	}
	return encodeWAV(stereo, recChannels, recSampleRate, recBitDepth)
}

// CustomerMonoWAV extracts the customer (left) channel from our PCM16 stereo
// recording. Never send the mixed call to post-call customer transcription:
// the agent channel can otherwise be attributed to the customer.
func CustomerMonoWAV(stereo []byte) ([]byte, error) {
	if len(stereo) < 44 || string(stereo[:4]) != "RIFF" || string(stereo[8:12]) != "WAVE" {
		return nil, errors.New("invalid WAV header")
	}
	var channels, sampleRate, bits uint32
	var data []byte
	for pos := 12; pos+8 <= len(stereo); {
		size := int(binary.LittleEndian.Uint32(stereo[pos+4 : pos+8]))
		start := pos + 8
		if size < 0 || size > len(stereo)-start {
			return nil, errors.New("invalid WAV chunk size")
		}
		switch string(stereo[pos : pos+4]) {
		case "fmt ":
			if size < 16 || binary.LittleEndian.Uint16(stereo[start:start+2]) != 1 {
				return nil, errors.New("unsupported WAV format")
			}
			channels = uint32(binary.LittleEndian.Uint16(stereo[start+2 : start+4]))
			sampleRate = binary.LittleEndian.Uint32(stereo[start+4 : start+8])
			bits = uint32(binary.LittleEndian.Uint16(stereo[start+14 : start+16]))
		case "data":
			data = stereo[start : start+size]
		}
		pos = start + size + size%2
	}
	if channels != 2 || sampleRate != recSampleRate || bits != recBitDepth || len(data) == 0 || len(data)%4 != 0 {
		return nil, errors.New("unsupported stereo WAV layout")
	}
	mono := make([]byte, len(data)/2)
	for i := 0; i < len(mono)/2; i++ {
		copy(mono[i*2:i*2+2], data[i*4:i*4+2])
	}
	return encodeWAV(mono, 1, recSampleRate, recBitDepth), nil
}

// pcmPeak returns the absolute peak sample value of a PCM16LE buffer.
func pcmPeak(buf []byte) int {
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
	return int(peak)
}

// normalizeTo scales buf so its peak matches target.
// Only amplifies — never attenuates (if buf is already at or above target, no change).
func normalizeTo(buf []byte, target int) {
	if len(buf) < 2 || target <= 0 {
		return
	}
	peak := pcmPeak(buf)
	if peak == 0 || peak >= target {
		return
	}
	gain := float32(target) / float32(peak)
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
	binary.Write(&buf, binary.LittleEndian, uint32(16))         //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(channels))   //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate)) //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint32(byteRate))   //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint16(blockAlign)) //nolint:errcheck
	binary.Write(&buf, binary.LittleEndian, uint16(bitDepth))   //nolint:errcheck
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataLen)) //nolint:errcheck
	buf.Write(data)
	return buf.Bytes()
}
