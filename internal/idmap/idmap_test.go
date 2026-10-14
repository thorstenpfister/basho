package idmap

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestMap(t *testing.T) {
	m := New([]domain.Rikishi{{ID: 8850, NSKID: 4227}, {ID: 3081}}) // Onosato; Hakuho without a JSA ID here
	if id, ok := m.ByJSA(4227); !ok || id != 8850 {
		t.Errorf("4227 → %d, %v", id, ok)
	}
	if _, ok := m.ByJSA(0); ok || m.Len() != 1 {
		t.Error("an entry without a JSA ID was mapped")
	}
	m.Add([]domain.Rikishi{{ID: 9000, NSKID: 4227}})
	if id, _ := m.ByJSA(4227); id != 9000 {
		t.Errorf("update: %d", id)
	}
}
