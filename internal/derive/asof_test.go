package derive

import (
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

var (
	aki  = domain.BashoID{Year: 2026, Month: time.September}
	nago = domain.BashoID{Year: 2026, Month: time.July}
)

func bout(basho domain.BashoID, div domain.Division, day, n int) domain.Bout {
	return domain.Bout{Basho: basho, Division: div, Day: day, MatchNo: n}
}

func TestAsOfIncludes(t *testing.T) {
	m13 := bout(aki, domain.DivMakuuchi, 13, 10)
	playoff := bout(aki, domain.DivMakuuchi, 15, 1)
	playoff.IsPlayoff = true
	tests := []struct {
		name string
		c    AsOf
		b    domain.Bout
		want bool
	}{
		{"earlier tournament", BeforeDay(aki, 1), bout(nago, domain.DivMakuuchi, 15, 20), true},
		{"later tournament", EndOf(nago), bout(aki, domain.DivJuryo, 1, 1), false},
		{"before the tournament", BeforeDay(aki, 1), bout(aki, domain.DivJuryo, 1, 1), false},
		{"earlier day", BeforeDay(aki, 13), bout(aki, domain.DivMakuuchi, 12, 20), true},
		{"start of the day", BeforeDay(aki, 13), m13, false},
		{"end of the day", AfterDay(aki, 13), m13, true},
		{"next day", AfterDay(aki, 13), bout(aki, domain.DivJuryo, 14, 1), false},
		{"the bout itself, before", BeforeBout(m13), m13, false},
		{"the bout itself, through", ThroughBout(m13), m13, true},
		{"earlier match", BeforeBout(m13), bout(aki, domain.DivMakuuchi, 13, 9), true},
		{"later match", ThroughBout(m13), bout(aki, domain.DivMakuuchi, 13, 11), false},
		{"Juryo fights before Makuuchi", BeforeBout(m13), bout(aki, domain.DivJuryo, 13, 14), true},
		{"Makuuchi after a Juryo cutoff", ThroughBout(bout(aki, domain.DivJuryo, 13, 14)), bout(aki, domain.DivMakuuchi, 13, 1), false},
		{"playoff after the regular bouts", BeforeBout(playoff), bout(aki, domain.DivMakuuchi, 15, 20), true},
		{"playoff not before its own regular bouts", BeforeBout(bout(aki, domain.DivMakuuchi, 15, 20)), playoff, false},
		{"playoff at the end of day 15", EndOf(aki), playoff, true},
		{"everything", NoCutoff, bout(domain.BashoID{Year: 2100, Month: time.January}, domain.DivMakuuchi, 15, 1), true},
		{"zero value", AsOf{}, bout(domain.FirstBasho, domain.DivJuryo, 1, 1), false},
	}
	for _, tt := range tests {
		if got := tt.c.Includes(tt.b); got != tt.want {
			t.Errorf("%s: Includes = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAsOfIncludesDay(t *testing.T) {
	m13 := bout(aki, domain.DivMakuuchi, 13, 10)
	tests := []struct {
		name string
		c    AsOf
		day  int
		b    *domain.Bout
		want bool
	}{
		{"by its bout", BeforeBout(m13), 13, &m13, false},
		{"by its bout, through", ThroughBout(m13), 13, &m13, true},
		{"absent, the day has started", BeforeBout(m13), 13, nil, true},
		{"absent, start of the day", BeforeDay(aki, 13), 13, nil, false},
		{"absent, end of the day", AfterDay(aki, 13), 13, nil, true},
		{"absent, earlier day", BeforeDay(aki, 13), 12, nil, true},
		{"absent, later day", AfterDay(aki, 13), 14, nil, false},
	}
	for _, tt := range tests {
		if got := tt.c.IncludesDay(aki, tt.day, tt.b); got != tt.want {
			t.Errorf("%s: IncludesDay = %v, want %v", tt.name, got, tt.want)
		}
	}
	if !BeforeDay(aki, 1).IncludesDay(nago, 15, nil) || AfterDay(nago, 15).IncludesDay(aki, 1, nil) {
		t.Error("tournament order ignored")
	}
}

func TestBefore(t *testing.T) {
	var career []*domain.Bout
	for _, b := range []domain.Bout{
		bout(nago, domain.DivJuryo, 14, 3), bout(nago, domain.DivJuryo, 15, 2),
		bout(aki, domain.DivMakuuchi, 1, 1), bout(aki, domain.DivMakuuchi, 2, 4), bout(aki, domain.DivMakuuchi, 3, 2),
	} {
		career = append(career, &b)
	}
	for _, tt := range []struct {
		c    AsOf
		want int
	}{
		{BeforeDay(aki, 1), 2},
		{BeforeBout(*career[3]), 3},
		{ThroughBout(*career[3]), 4},
		{NoCutoff, 5},
		{AsOf{}, 0},
	} {
		if got := len(before(career, tt.c)); got != tt.want {
			t.Errorf("before(%+v) = %d bouts, want %d", tt.c, got, tt.want)
		}
	}
}

func TestComparePlay(t *testing.T) {
	playoff := bout(aki, domain.DivMakuuchi, 15, 1)
	playoff.IsPlayoff = true
	ordered := []domain.Bout{
		bout(nago, domain.DivMakuuchi, 15, 20),
		bout(aki, domain.DivJuryo, 1, 14),
		bout(aki, domain.DivMakuuchi, 1, 1),
		bout(aki, domain.DivMakuuchi, 1, 2),
		bout(aki, domain.DivMakuuchi, 15, 20),
		playoff,
	}
	for i := 1; i < len(ordered); i++ {
		if ComparePlay(ordered[i-1], ordered[i]) >= 0 || ComparePlay(ordered[i], ordered[i-1]) <= 0 {
			t.Errorf("bout %d not before bout %d", i-1, i)
		}
	}
}

func TestEarlier(t *testing.T) {
	aki := domain.BashoID{Year: 2026, Month: 9}
	j5 := domain.Bout{Basho: aki, Day: 13, Division: domain.DivJuryo, MatchNo: 5}
	m5 := domain.Bout{Basho: aki, Day: 13, Division: domain.DivMakuuchi, MatchNo: 5}
	m6 := m5
	m6.MatchNo = 6
	ordered := []AsOf{
		BeforeDay(aki, 1), AfterDay(aki, 12), BeforeDay(aki, 13),
		BeforeBout(j5), ThroughBout(j5), BeforeBout(m5), ThroughBout(m5), BeforeBout(m6),
		AfterDay(aki, 13), EndOf(aki), BeforeDay(aki.Next(), 1), NoCutoff,
	}
	for i := range ordered {
		for j := range ordered {
			want := ordered[min(i, j)]
			// Equal points (end of day 12, start of day 13) may come back either way.
			if got := Earlier(ordered[i], ordered[j]); got != want && got.point().compare(want.point()) != 0 {
				t.Errorf("Earlier(%d, %d) = %+v; want %+v", i, j, got, want)
			}
			if after := ordered[i].After(ordered[j]); after != (i > j) && ordered[i].point().compare(ordered[j].point()) != 0 {
				t.Errorf("%d after %d: %v", i, j, after)
			}
		}
	}
}
