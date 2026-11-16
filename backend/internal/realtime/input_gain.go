package realtime

import (
	"encoding/binary"
	"math"
)

const (
	inputGainTargetRMS  = 1800.0
	inputGainMaximum    = 6.0 // approximately +15.6 dB, never unbounded normalization
	inputGainPeak       = 26000.0
	inputGainMinimumRMS = 60.0
)

// inputGainController is per-call and owned by the Live writer loop. It does
// not gate speech, classify words, buffer audio, or change the sample count.
// Short 10 ms windows provide fast adjustment for brief answers. The energy
// and noise floor checks avoid raising digital silence and low background
// noise; they are not a semantic speech detector.
type inputGainController struct {
	gain       float64
	noiseFloor float64
}

func newInputGainController() *inputGainController {
	return &inputGainController{gain: 1, noiseFloor: 20}
}

func (g *inputGainController) process(pcm []byte) []byte {
	if g == nil || len(pcm) == 0 || len(pcm)%2 != 0 {
		return pcm
	}
	out := make([]byte, len(pcm))
	for offset := 0; offset < len(pcm); offset += 160 {
		frame := pcm[offset:min(offset+160, len(pcm))]
		var energy, peak float64
		for i := 0; i < len(frame); i += 2 {
			x := float64(int16(binary.LittleEndian.Uint16(frame[i:])))
			energy += x * x
			peak = math.Max(peak, math.Abs(x))
		}
		rms := math.Sqrt(energy / float64(len(frame)/2))
		duration := float64(len(frame)/2) / 8000
		// Follow low background levels without treating clear speech peaks as
		// an increase in the noise floor.
		if rms < g.noiseFloor*2 {
			alpha := 1 - math.Exp(-duration)
			g.noiseFloor = math.Max(10, g.noiseFloor+alpha*(rms-g.noiseFloor))
		}
		floor := math.Max(inputGainMinimumRMS, 3*g.noiseFloor)
		desired := 1.0
		if rms > floor {
			desired = math.Min(inputGainMaximum, inputGainTargetRMS/rms)
			// Blend at the noise boundary instead of switching gain abruptly.
			if desired > 1 {
				desired = 1 + (desired-1)*math.Min(1, (rms-floor)/floor)
			}
		}
		previous := g.gain
		seconds := 0.02 // fast gain increase for one-word answers
		if desired < previous {
			seconds = 0.01
		}
		g.gain = previous + (desired-previous)*(1-math.Exp(-duration/seconds))
		limit := 1.0
		if peak > 0 {
			limit = inputGainPeak / peak
		}
		startGain, endGain := math.Min(previous, limit), math.Min(g.gain, limit)
		// Do not amplify silence/noise even if the preceding speech needed gain.
		if rms <= floor {
			startGain = math.Min(1, startGain)
			endGain = math.Min(1, endGain)
		}
		for i := 0; i < len(frame); i += 2 {
			fraction := float64(i/2+1) / float64(len(frame)/2)
			gain := startGain + (endGain-startGain)*fraction
			x := float64(int16(binary.LittleEndian.Uint16(frame[i:])))
			y := math.Round(x * gain)
			y = math.Max(-inputGainPeak, math.Min(inputGainPeak, y))
			binary.LittleEndian.PutUint16(out[offset+i:], uint16(int16(y)))
		}
	}
	return out
}
