package derive

import (
	"maps"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestCareerAt(t *testing.T) {
	may := domain.BashoID{Year: 2026, Month: time.May}
	r := domain.Rikishi{
		ID: 7, Name: domain.Name{En: "Onosato"}, Birth: domain.NewDate(2000, time.June, 7),
		Height: 192, Weight: 191,
	}
	yusho := func(id domain.BashoID, mak, jur domain.RikishiID, prizes ...domain.Prize) domain.Basho {
		return domain.Basho{ID: id, Yusho: map[domain.Division]domain.RikishiID{domain.DivMakuuchi: mak, domain.DivJuryo: jur, domain.DivMakushita: 7}, Prizes: prizes}
	}
	tournaments := []domain.Basho{
		yusho(may, 9, 7, domain.Prize{Type: "Kanto-sho", RikishiID: 7}),
		yusho(nago, 7, 9, domain.Prize{Type: "Gino-sho", RikishiID: 7}, domain.Prize{Type: "Kanto-sho", RikishiID: 3}),
		yusho(aki, 7, 9, domain.Prize{Type: "Kanto-sho", RikishiID: 7}),
	}
	akiMeta := domain.Basho{ID: aki, Start: domain.NewDate(2026, time.September, 13)}

	tests := []struct {
		name   string
		c      AsOf
		yusho  map[domain.Division]int
		sansho map[string]int
	}{
		{"during day 15 of Aki", ThroughBout(bout(aki, domain.DivMakuuchi, 15, 18)), map[domain.Division]int{domain.DivMakuuchi: 1, domain.DivJuryo: 1}, map[string]int{"Kanto-sho": 1, "Gino-sho": 1}},
		{"at the end of Aki", EndOf(aki), map[domain.Division]int{domain.DivMakuuchi: 2, domain.DivJuryo: 1}, map[string]int{"Kanto-sho": 2, "Gino-sho": 1}},
		{"before May", BeforeDay(may, 1), map[domain.Division]int{}, map[string]int{}},
	}
	for _, tt := range tests {
		st := CareerAt(r, domain.Name{En: "Onosato"}, akiMeta, tournaments, tt.c, aki)
		if !maps.Equal(st.Yusho, tt.yusho) || !maps.Equal(st.Sansho, tt.sansho) {
			t.Errorf("%s: yusho %v sansho %v; want %v %v", tt.name, st.Yusho, st.Sansho, tt.yusho, tt.sansho)
		}
	}

	st := CareerAt(r, domain.Name{En: "Onosato"}, akiMeta, tournaments, NoCutoff, aki)
	if st.Age != 26 || st.Height != 192 || st.Latest || st.Now != "" {
		t.Errorf("current tournament: %+v", st)
	}
	// An old tournament under an earlier shikona: latest measurements, "now".
	old := domain.Basho{ID: may, Start: domain.NewDate(2026, time.May, 10)}
	st = CareerAt(r, domain.Name{En: "Oshita"}, old, tournaments, BeforeDay(may, 1), aki)
	if st.Age != 25 || !st.Latest || st.Now != "Onosato" {
		t.Errorf("older tournament: %+v", st)
	}
}

func TestAge(t *testing.T) {
	birth := domain.NewDate(2000, time.September, 13)
	for _, tt := range []struct {
		on   domain.Date
		want int
	}{
		{domain.NewDate(2026, time.September, 12), 25},
		{domain.NewDate(2026, time.September, 13), 26},
		{domain.NewDate(2026, time.January, 1), 25},
	} {
		if got := age(birth, tt.on); got != tt.want {
			t.Errorf("age on %v = %d, want %d", tt.on, got, tt.want)
		}
	}
	if age(domain.Date{}, birth) != 0 {
		t.Error("unknown birth date")
	}
}
