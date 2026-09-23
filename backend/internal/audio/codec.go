// Package audio handles PCM/ulaw codec operations and resampling.
// Replaces Python's audioop.ulaw2lin, audioop.lin2ulaw, and audioop.ratecv.
package audio

import "github.com/zaf/g711"

// UlawToPCM converts 8-bit mu-law bytes to 16-bit linear PCM bytes (little-endian).
// Equivalent to: audioop.ulaw2lin(data, 2)
// g711.DecodeUlaw accepts a ulaw buffer and returns a PCM buffer.
func UlawToPCM(ulaw []byte) []byte {
	return g711.DecodeUlaw(ulaw)
}

// PCMToUlaw converts 16-bit linear PCM bytes (little-endian) to 8-bit mu-law bytes.
// Equivalent to: audioop.lin2ulaw(data, 2)
// g711.EncodeUlaw accepts a PCM buffer and returns a ulaw buffer.
func PCMToUlaw(pcm []byte) []byte {
	return g711.EncodeUlaw(pcm)
}

// Decimate2x downsamples 16kHz 16-bit PCM to 8kHz by dropping every other sample.
// Equivalent to: audioop.ratecv(data, 2, 1, 16000, 8000, None)
// Input must have an even number of bytes (pairs of int16 samples at 16kHz).
func Decimate2x(pcm16k []byte) []byte {
	samples := len(pcm16k) / 2 // total 16-bit samples at 16kHz
	outSamples := samples / 2  // keep every 2nd sample → 8kHz
	out := make([]byte, outSamples*2)
	for i := range outSamples {
		// Source sample index i*2 (take every other sample)
		src := i * 4
		out[i*2] = pcm16k[src]
		out[i*2+1] = pcm16k[src+1]
	}
	return out
}

// Upsample2x converts 8kHz PCM16LE to 16kHz using linear interpolation.
func Upsample2x(pcm8k []byte) []byte {
	if len(pcm8k) < 2 {
		return nil
	}
	samples := len(pcm8k) / 2
	out := make([]byte, samples*4)
	for i := 0; i < samples; i++ {
		cur := int16(uint16(pcm8k[i*2]) | uint16(pcm8k[i*2+1])<<8)
		next := cur
		if i+1 < samples {
			next = int16(uint16(pcm8k[(i+1)*2]) | uint16(pcm8k[(i+1)*2+1])<<8)
		}
		mid := int16((int32(cur) + int32(next)) / 2)
		for j, sample := range []int16{cur, mid} {
			off := (i*2 + j) * 2
			out[off] = byte(sample)
			out[off+1] = byte(uint16(sample) >> 8)
		}
	}
	return out
}

// Decimate3x converts Gemini Live's 24kHz PCM16LE output to telephony 8kHz.
func Decimate3x(pcm24k []byte) []byte {
	samples := len(pcm24k) / 2
	outSamples := samples / 3
	out := make([]byte, outSamples*2)
	for i := 0; i < outSamples; i++ {
		src := i * 6
		out[i*2] = pcm24k[src]
		out[i*2+1] = pcm24k[src+1]
	}
	return out
}
