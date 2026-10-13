package domain

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

// Side is the half of the banzuke a wrestler is listed on. The zero Side means
// "none", e.g. the winner of an undecided bout.
type Side int

const (
	East Side = iota + 1
	West
)

func (s Side) String() string {
	switch s {
	case East:
		return "East"
	case West:
		return "West"
	}
	return ""
}

// Division is a division of the banzuke. basho covers Makuuchi and Juryo;
// DivMakushita exists only for Makushita wrestlers fighting in Juryo bouts.
type Division int

const (
	DivMakuuchi Division = iota + 1
	DivJuryo
	DivMakushita
)

func (d Division) String() string {
	switch d {
	case DivMakuuchi:
		return "Makuuchi"
	case DivJuryo:
		return "Juryo"
	case DivMakushita:
		return "Makushita"
	}
	return ""
}

// ParseDivision parses the names of String, as sumo-api writes them.
func ParseDivision(s string) (Division, error) {
	for d := DivMakuuchi; d <= DivMakushita; d++ {
		if d.String() == s {
			return d, nil
		}
	}
	return 0, fmt.Errorf("unknown division %q", s)
}

// MarshalText implements encoding.TextMarshaler.
func (d Division) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Division) UnmarshalText(b []byte) (err error) {
	*d, err = ParseDivision(string(b))
	return err
}

// RankTitle is a rank within the banzuke, highest first.
type RankTitle int

const (
	Yokozuna RankTitle = iota + 1
	Ozeki
	Sekiwake
	Komusubi
	Maegashira
	Juryo
	Makushita
)

var titles = []struct {
	name, short string
	division    Division
}{
	Yokozuna:   {"Yokozuna", "Y", DivMakuuchi},
	Ozeki:      {"Ozeki", "O", DivMakuuchi},
	Sekiwake:   {"Sekiwake", "S", DivMakuuchi},
	Komusubi:   {"Komusubi", "K", DivMakuuchi},
	Maegashira: {"Maegashira", "M", DivMakuuchi},
	Juryo:      {"Juryo", "J", DivJuryo},
	Makushita:  {"Makushita", "Ms", DivMakushita},
}

func (t RankTitle) valid() bool { return t >= Yokozuna && t <= Makushita }

func (t RankTitle) String() string {
	if !t.valid() {
		return ""
	}
	return titles[t].name
}

// Sanyaku reports the titles above Maegashira.
func (t RankTitle) Sanyaku() bool { return t >= Yokozuna && t <= Komusubi }

// Division returns the division a title belongs to.
func (t RankTitle) Division() Division {
	if !t.valid() {
		return 0
	}
	return titles[t].division
}

// Rank is a banzuke position, e.g. Maegashira 17 East.
type Rank struct {
	Title  RankTitle
	Number int // 1-based within the title and side
	Side   Side
}

// ParseRank parses sumo-api's English form, "<Title> <N> <East|West>".
func ParseRank(s string) (Rank, error) {
	fields := strings.Fields(s)
	if len(fields) != 3 {
		return Rank{}, fmt.Errorf("rank %q: want \"<Title> <N> <East|West>\"", s)
	}
	var title RankTitle
	for t := Yokozuna; t <= Makushita; t++ {
		if strings.EqualFold(fields[0], titles[t].name) {
			title = t
			break
		}
	}
	if title == 0 {
		return Rank{}, fmt.Errorf("rank %q: unknown title %q", s, fields[0])
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n < 1 {
		return Rank{}, fmt.Errorf("rank %q: bad number %q", s, fields[1])
	}
	var side Side
	switch strings.ToLower(fields[2]) {
	case "east":
		side = East
	case "west":
		side = West
	default:
		return Rank{}, fmt.Errorf("rank %q: bad side %q", s, fields[2])
	}
	return Rank{title, n, side}, nil
}

// String returns the long form, e.g. "Maegashira 17 East".
func (r Rank) String() string {
	if !r.Title.valid() {
		return ""
	}
	return fmt.Sprintf("%s %d %s", r.Title, r.Number, r.Side)
}

// MarshalText implements encoding.TextMarshaler with the long form.
func (r Rank) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler; empty text is the zero Rank.
func (r *Rank) UnmarshalText(b []byte) (err error) {
	if len(b) == 0 {
		*r = Rank{}
		return nil
	}
	*r, err = ParseRank(string(b))
	return err
}

// Short returns the compact form, e.g. "M17e", "Y1w", "Ms3e".
func (r Rank) Short() string {
	if !r.Title.valid() {
		return ""
	}
	side := r.Side.String()
	if side != "" {
		side = strings.ToLower(side[:1])
	}
	return titles[r.Title].short + strconv.Itoa(r.Number) + side
}

// Compare orders ranks from highest (Yokozuna 1 East) to lowest: by title,
// then number, then East before West. It returns -1 if r ranks higher than o.
func (r Rank) Compare(o Rank) int {
	return cmp.Or(cmp.Compare(r.Title, o.Title), cmp.Compare(r.Number, o.Number), cmp.Compare(r.Side, o.Side))
}
