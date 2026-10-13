package data

import "testing"

func TestSnapshotOpens(t *testing.T) {
	s, err := Snapshot()
	if err != nil {
		t.Fatalf("the embedded snapshot does not open: %v", err)
	}
	if _, err := s.Rikishi(); err != nil {
		t.Errorf("Rikishi: %v", err)
	}
}
