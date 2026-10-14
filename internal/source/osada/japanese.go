package osada

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Japanese rank titles, highest first (SPEC Appendix C).
var rankTitles = []struct {
	jp    string
	title domain.RankTitle
}{
	{"横綱", domain.Yokozuna},
	{"大関", domain.Ozeki},
	{"関脇", domain.Sekiwake},
	{"小結", domain.Komusubi},
	{"前頭", domain.Maegashira},
	{"十両", domain.Juryo},
	{"幕下", domain.Makushita},
}

// parseRank parses osada.us's Japanese ranks: "横綱", "前頭十七枚目",
// "前頭筆頭", "十両2" (the banzuke's form), with an optional "張出"
// (haridashi) prefix. A rank without a number is number 1; the side is not
// part of the text.
func parseRank(s string) (domain.Rank, error) {
	rest := strings.TrimPrefix(strings.TrimSpace(s), "張出")
	for _, t := range rankTitles {
		num, ok := strings.CutPrefix(rest, t.jp)
		if !ok {
			continue
		}
		num = strings.TrimSuffix(strings.TrimPrefix(num, "張出"), "枚目")
		n := 1
		if num != "" && num != "筆頭" { // 筆頭: the head, number 1
			var err error
			if n, err = parseNumber(num); err != nil {
				return domain.Rank{}, fmt.Errorf("rank %q: %w", s, err)
			}
		}
		return domain.Rank{Title: t.title, Number: n}, nil
	}
	return domain.Rank{}, fmt.Errorf("rank %q: unknown title", s)
}

var kanjiDigits = map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}

// parseNumber parses a number in kanji (十七, 二十, 百五) or ASCII digits,
// up to 999.
func parseNumber(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n, nil
	}
	n, digit, last := 0, 0, 1000
	for _, r := range s {
		switch r {
		case '百', '十':
			unit := 10
			if r == '百' {
				unit = 100
			}
			if unit >= last { // 十十, 十百
				return 0, fmt.Errorf("bad number %q", s)
			}
			n += max(digit, 1) * unit
			digit, last = 0, unit
		default:
			d, ok := kanjiDigits[r]
			if !ok || digit != 0 {
				return 0, fmt.Errorf("bad number %q", s)
			}
			digit = d
		}
	}
	if n+digit == 0 {
		return 0, fmt.Errorf("bad number %q", s)
	}
	return n + digit, nil
}

// reiwa is the year before Reiwa 1 (2019).
const reiwa = 2018

// parseBasho reads a tournament from osada.us's banzuke heading:
// year "令和八年" and name "九月場所".
func parseBasho(year, name string) (domain.BashoID, error) {
	y, ok := strings.CutPrefix(year, "令和")
	y, ok2 := strings.CutSuffix(y, "年")
	if !ok || !ok2 {
		return domain.BashoID{}, fmt.Errorf("year %q: not a Reiwa year", year)
	}
	n := 1
	if y != "元" {
		var err error
		if n, err = parseNumber(y); err != nil {
			return domain.BashoID{}, fmt.Errorf("year %q: %w", year, err)
		}
	}
	m, ok := strings.CutSuffix(name, "月場所")
	if !ok {
		return domain.BashoID{}, fmt.Errorf("tournament %q: want \"<month>月場所\"", name)
	}
	month, err := parseNumber(m)
	if err != nil || month > 12 {
		return domain.BashoID{}, fmt.Errorf("tournament %q: bad month", name)
	}
	id, err := domain.ParseBashoID(fmt.Sprintf("%04d%02d", reiwa+n, month))
	if err != nil {
		return domain.BashoID{}, fmt.Errorf("tournament %s %s: %w", year, name, err)
	}
	return id, nil
}

// undecided is osada.us's kimarite of a bout still to come.
const undecided = "未定"

// spellings are the JSA's spellings that differ from the kimarite table's.
var spellings = map[string]domain.Kimarite{
	"浴せ倒し": "abisetaoshi", // 浴びせ倒し
}

// kimarite maps a Japanese technique to its key; unknown names pass through
// unchanged (SPEC §5.6), undecided is empty.
func kimarite(s string) domain.Kimarite {
	s = strings.TrimSpace(s)
	if s == "" || s == undecided {
		return ""
	}
	if k, ok := domain.KimariteByKanji(s); ok {
		return k
	}
	if k, ok := spellings[s]; ok {
		return k
	}
	return domain.Kimarite(s)
}
