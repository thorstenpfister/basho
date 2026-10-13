package atomicfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if err := Write(path, []byte{byte('a' + i%26)}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	b, err := os.ReadFile(path)
	if err != nil || len(b) != 1 {
		t.Fatalf("content = %q, %v; want one complete write", b, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("files = %d, want no leftover temporary files", len(entries))
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}
