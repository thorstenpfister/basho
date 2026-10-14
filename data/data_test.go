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

func TestRatingParams(t *testing.T) {
	f, err := RatingParams()
	if err != nil {
		t.Fatal(err)
	}
	if f.Params.K <= 0 || f.Split.Through.IsZero() {
		t.Errorf("params %+v", f)
	}
}
