package idmap

import (
	"strings"
	"unicode"
)

// kana is modified Hepburn for hiragana, without macrons (SPEC §5.6).
var kana = map[string]string{
	"あ": "a", "い": "i", "う": "u", "え": "e", "お": "o",
	"か": "ka", "き": "ki", "く": "ku", "け": "ke", "こ": "ko",
	"さ": "sa", "し": "shi", "す": "su", "せ": "se", "そ": "so",
	"た": "ta", "ち": "chi", "つ": "tsu", "て": "te", "と": "to",
	"な": "na", "に": "ni", "ぬ": "nu", "ね": "ne", "の": "no",
	"は": "ha", "ひ": "hi", "ふ": "fu", "へ": "he", "ほ": "ho",
	"ま": "ma", "み": "mi", "む": "mu", "め": "me", "も": "mo",
	"や": "ya", "ゆ": "yu", "よ": "yo",
	"ら": "ra", "り": "ri", "る": "ru", "れ": "re", "ろ": "ro",
	"わ": "wa", "ゐ": "i", "ゑ": "e", "を": "o", "ん": "n",
	"が": "ga", "ぎ": "gi", "ぐ": "gu", "げ": "ge", "ご": "go",
	"ざ": "za", "じ": "ji", "ず": "zu", "ぜ": "ze", "ぞ": "zo",
	"だ": "da", "ぢ": "ji", "づ": "zu", "で": "de", "ど": "do",
	"ば": "ba", "び": "bi", "ぶ": "bu", "べ": "be", "ぼ": "bo",
	"ぱ": "pa", "ぴ": "pi", "ぷ": "pu", "ぺ": "pe", "ぽ": "po",
	"ゔ":  "vu",
	"きゃ": "kya", "きゅ": "kyu", "きょ": "kyo",
	"しゃ": "sha", "しゅ": "shu", "しょ": "sho",
	"ちゃ": "cha", "ちゅ": "chu", "ちょ": "cho",
	"にゃ": "nya", "にゅ": "nyu", "にょ": "nyo",
	"ひゃ": "hya", "ひゅ": "hyu", "ひょ": "hyo",
	"みゃ": "mya", "みゅ": "myu", "みょ": "myo",
	"りゃ": "rya", "りゅ": "ryu", "りょ": "ryo",
	"ぎゃ": "gya", "ぎゅ": "gyu", "ぎょ": "gyo",
	"じゃ": "ja", "じゅ": "ju", "じょ": "jo",
	"びゃ": "bya", "びゅ": "byu", "びょ": "byo",
	"ぴゃ": "pya", "ぴゅ": "pyu", "ぴょ": "pyo",
	"ぁ": "a", "ぃ": "i", "ぅ": "u", "ぇ": "e", "ぉ": "o",
	"ゃ": "ya", "ゅ": "yu", "ょ": "yo",
}

// longVowels collapses the long vowels: おう and おお are o, うう is u.
var longVowels = strings.NewReplacer("ou", "o", "oo", "o", "uu", "u")

// Romaji transliterates a kana name, "おおのさと" → "Onosato" (SPEC §5.6).
// It is known to be imperfect ("しょうなんのうみ" gives "Shonannomi" for
// Shonannoumi), so callers mark the result as approximate. Katakana is read
// as hiragana; anything else is dropped.
func Romaji(s string) string {
	rs := []rune(toHiragana(s))
	var b strings.Builder
	double := false // after っ: double the next consonant
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 'っ' {
			double = true
			continue
		}
		var syl string
		if i+1 < len(rs) {
			if v, ok := kana[string(rs[i:i+2])]; ok {
				syl, i = v, i+1
			}
		}
		if syl == "" {
			syl = kana[string(r)]
		}
		if syl == "" {
			continue // ー, punctuation
		}
		if double {
			if strings.HasPrefix(syl, "ch") {
				b.WriteByte('t') // っち: tchi
			} else {
				b.WriteByte(syl[0])
			}
			double = false
		}
		b.WriteString(syl)
	}
	out := longVowels.Replace(b.String())
	if out == "" {
		return ""
	}
	return string(unicode.ToUpper(rune(out[0]))) + out[1:]
}

// toHiragana maps katakana to hiragana.
func toHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 'ァ' + 'ぁ'
		}
		return r
	}, s)
}
