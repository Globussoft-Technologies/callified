package wshandler

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// Optional private, bounded PCM pairs for comparing Gemini output with conversion.
// Each capture contains at most 60 seconds of generated audio, not wall time.
// These files are not carrier recordings: rejected turns may also be included.
type liveAudioDiagnostic struct {
	raw, converted *os.File
	written        int
	log            *zap.Logger
}

func newLiveAudioDiagnostic(dir string, log *zap.Logger) *liveAudioDiagnostic {
	d := &liveAudioDiagnostic{log: log}
	if dir == "" {
		return d
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Warn("live audio diagnostic unavailable", zap.Error(err))
		return d
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) >= 5 {
		return d
	} // At most five calls per diagnostic directory.
	capture, err := os.MkdirTemp(dir, "call-")
	if err != nil {
		log.Warn("live audio diagnostic unavailable", zap.Error(err))
		return d
	}
	d.raw, err = os.OpenFile(filepath.Join(capture, "gemini-24000.pcm"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		d.converted, err = os.OpenFile(filepath.Join(capture, "telephony-8000.pcm"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if err != nil {
		d.close()
		log.Warn("live audio diagnostic unavailable", zap.Error(err))
		return d
	}
	log.Info("live audio diagnostic enabled", zap.String("capture", capture), zap.String("format", "mono PCM16LE"))
	return d
}

func (d *liveAudioDiagnostic) write(raw, converted []byte) {
	if d.raw == nil || d.converted == nil {
		return
	}
	if d.written+len(raw) > 24000*2*60 {
		d.close()
		return
	}
	if _, err := d.raw.Write(raw); err != nil {
		d.log.Warn("live audio diagnostic write failed", zap.Error(err))
		d.close()
		return
	}
	if _, err := d.converted.Write(converted); err != nil {
		d.log.Warn("live audio diagnostic write failed", zap.Error(err))
		d.close()
		return
	}
	d.written += len(raw)
}
func (d *liveAudioDiagnostic) close() {
	if d.raw != nil {
		_ = d.raw.Close()
		d.raw = nil
	}
	if d.converted != nil {
		_ = d.converted.Close()
		d.converted = nil
	}
}
