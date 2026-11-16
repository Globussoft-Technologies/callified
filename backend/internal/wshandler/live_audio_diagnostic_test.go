package wshandler

import (
	"os"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestLiveAudioDiagnosticConcurrentCaptureLimit(t *testing.T) {
	dir := t.TempDir()
	start := make(chan struct{})
	var calls sync.WaitGroup
	for i := 0; i < 64; i++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			<-start
			d := newLiveAudioDiagnostic(dir, zap.NewNop())
			d.write([]byte{1, 0}, []byte{1, 0})
			d.close()
		}()
	}
	close(start)
	calls.Wait()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d captures, want exactly 5", len(entries))
	}
	for _, entry := range entries {
		files, err := os.ReadDir(dir + "/" + entry.Name())
		if err != nil || len(files) != 2 {
			t.Fatalf("incomplete capture %s: files=%d, err=%v", entry.Name(), len(files), err)
		}
	}
}
