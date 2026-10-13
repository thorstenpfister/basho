package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseBashoID(t *testing.T) {
	valid := map[string]BashoID{
		"196001": {1960, time.January},
		"202611": {2026, time.November},
	}
	for s, want := range valid {
		got, err := ParseBashoID(s)
		if err != nil || got != want {
			t.Errorf("ParseBashoID(%q) = %v, %v; want %v", s, got, err, want)
		}
		if got.String() != s {
			t.Errorf("String() = %q, want %q", got.String(), s)
		}
	}
	for _, s := range []string{"202610", "195911", "202613", "202600", "20261", "2026-11", "abcdef", "-20611"} {
		if id, err := ParseBashoID(s); err == nil {
			t.Errorf("ParseBashoID(%q) = %v, want an error", s, id)
		}
	}
}

func TestBashoIDNextPrev(t *testing.T) {
	nov := BashoID{2026, time.November}
	if got := nov.Next(); got != (BashoID{2027, time.January}) {
		t.Errorf("Next = %v", got)
	}
	if got := nov.Next().Prev(); got != nov {
		t.Errorf("Next().Prev() = %v", got)
	}
	if got := (BashoID{2026, time.September}).Next(); got != nov {
		t.Errorf("Next = %v", got)
	}
	if nov.Compare(nov.Next()) != -1 || nov.Next().Compare(nov) != 1 || nov.Compare(nov) != 0 {
		t.Error("Compare")
	}
}

func TestBashoIDJSON(t *testing.T) {
	m := map[BashoID]string{{2026, time.September}: "Onosato"}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"202609":"Onosato"}` {
		t.Errorf("marshal = %s", b)
	}
	var back map[BashoID]string
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back[BashoID{2026, time.September}] != "Onosato" {
		t.Errorf("round trip = %v", back)
	}
	// The zero BashoID means "unknown" and round-trips as "", like Date.
	type profile struct {
		Debut BashoID `json:"debut"`
	}
	b, err = json.Marshal(profile{})
	if err != nil || string(b) != `{"debut":""}` {
		t.Errorf("marshal zero = %s, %v", b, err)
	}
	var p profile
	if err := json.Unmarshal(b, &p); err != nil || !p.Debut.IsZero() {
		t.Errorf("unmarshal zero = %+v, %v", p, err)
	}

	var bad BashoID
	if err := json.Unmarshal([]byte(`"202610"`), &bad); err == nil {
		t.Error("unmarshal accepted an even month")
	}
}
