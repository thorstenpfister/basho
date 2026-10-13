package domain

import "strings"

// KimariteCategory groups winning techniques by fighting style (SPEC §6.7).
// It is basho's own grouping, not the Japan Sumo Association's.
type KimariteCategory int

const (
	Push        KimariteCategory = iota + 1 // oshi/tsuki: pushing and thrusting
	Belt                                    // yotsu: forcing out or down on the belt
	Throw                                   // throws, twists, trips, backward body drops
	Pull                                    // pulling and slapping down
	Other                                   // rare techniques and the opponent's own mistakes
	NoTechnique                             // fusen and draws: not a win by technique
)

var categoryNames = [...]string{"", "push", "belt", "throw", "pull", "other", "excluded"}

func (c KimariteCategory) String() string {
	if c < 0 || int(c) >= len(categoryNames) {
		return ""
	}
	return categoryNames[c]
}

// KimariteInfo describes a technique.
type KimariteInfo struct {
	Kanji    string
	English  string
	Category KimariteCategory
}

// kimariteTable lists the 82 official techniques, the non-techniques, and the
// historical values sumo-api.com uses (SPEC Appendix B, V11).
var kimariteTable = map[Kimarite]KimariteInfo{
	"oshidashi":        {"押し出し", "frontal push out", Push},
	"oshitaoshi":       {"押し倒し", "frontal push down", Push},
	"tsukidashi":       {"突き出し", "frontal thrust out", Push},
	"tsukitaoshi":      {"突き倒し", "frontal thrust down", Push},
	"okuridashi":       {"送り出し", "rear push out", Push},
	"okuritaoshi":      {"送り倒し", "rear push down", Push},
	"yorikiri":         {"寄り切り", "frontal force out", Belt},
	"yoritaoshi":       {"寄り倒し", "frontal crush out", Belt},
	"abisetaoshi":      {"浴びせ倒し", "backward force down", Belt},
	"kimedashi":        {"極め出し", "arm barring force out", Belt},
	"kimetaoshi":       {"極め倒し", "arm barring force down", Belt},
	"tsuridashi":       {"吊り出し", "frontal lift out", Belt},
	"tsuriotoshi":      {"吊り落とし", "frontal lift body slam", Belt},
	"okuritsuridashi":  {"送り吊り出し", "rear lift out", Belt},
	"okuritsuriotoshi": {"送り吊り落とし", "rear lift body slam", Belt},
	"waridashi":        {"割り出し", "upper-arm force out", Belt},
	"sabaori":          {"鯖折り", "forward force down", Belt},
	"uwatenage":        {"上手投げ", "overarm throw", Throw},
	"shitatenage":      {"下手投げ", "underarm throw", Throw},
	"sukuinage":        {"掬い投げ", "beltless arm throw", Throw},
	"kotenage":         {"小手投げ", "armlock throw", Throw},
	"kubinage":         {"首投げ", "headlock throw", Throw},
	"koshinage":        {"腰投げ", "hip throw", Throw},
	"ipponzeoi":        {"一本背負い", "one-armed shoulder throw", Throw},
	"kakenage":         {"掛け投げ", "hooking inner thigh throw", Throw},
	"nichonage":        {"二丁投げ", "body drop throw", Throw},
	"tsukaminage":      {"掴み投げ", "lifting overarm throw", Throw},
	"uwatedashinage":   {"上手出し投げ", "pulling overarm throw", Throw},
	"shitatedashinage": {"下手出し投げ", "pulling underarm throw", Throw},
	"yaguranage":       {"櫓投げ", "inner thigh lifting throw", Throw},
	"okurinage":        {"送り投げ", "rear throw down", Throw},
	"okurigake":        {"送り掛け", "rear leg trip", Throw},
	"ashitori":         {"足取り", "leg pick", Throw},
	"chongake":         {"ちょん掛け", "pulling heel hook", Throw},
	"kawazugake":       {"河津掛け", "hooking backward counter throw", Throw},
	"kekaeshi":         {"蹴返し", "minor inner foot sweep", Throw},
	"ketaguri":         {"蹴手繰り", "leg kick pull down", Throw},
	"kirikaeshi":       {"切り返し", "twisting backward knee trip", Throw},
	"komatasukui":      {"小股掬い", "over thigh scooping body drop", Throw},
	"kozumatori":       {"小褄取り", "ankle pick", Throw},
	"mitokorozeme":     {"三所攻め", "triple attack force out", Throw},
	"nimaigeri":        {"二枚蹴り", "ankle kicking twist down", Throw},
	"omata":            {"大股", "thigh scooping body drop", Throw},
	"sotogake":         {"外掛け", "outside leg trip", Throw},
	"sotokomata":       {"外小股", "over thigh scooping body drop", Throw},
	"susoharai":        {"裾払い", "rear foot sweep", Throw},
	"susotori":         {"裾取り", "ankle pick", Throw},
	"tsumatori":        {"褄取り", "rear toe pick", Throw},
	"uchigake":         {"内掛け", "inside leg trip", Throw},
	"watashikomi":      {"渡し込み", "thigh grabbing push down", Throw},
	"izori":            {"居反り", "backwards body drop", Throw},
	"kakezori":         {"掛け反り", "hooking backwards body drop", Throw},
	"shumokuzori":      {"撞木反り", "bell hammer backwards body drop", Throw},
	"sototasukizori":   {"外襷反り", "outer reverse backwards body drop", Throw},
	"tasukizori":       {"襷反り", "reverse backwards body drop", Throw},
	"tsutaezori":       {"伝え反り", "underarm forward body drop", Throw},
	"amiuchi":          {"網打ち", "fisherman's throw", Throw},
	"gasshohineri":     {"合掌捻り", "clasped-hand twist down", Throw},
	"harimanage":       {"波離間投げ", "backward belt throw", Throw},
	"kainahineri":      {"腕捻り", "two-handed arm twist down", Throw},
	"kotehineri":       {"小手捻り", "arm-locking twist down", Throw},
	"kubihineri":       {"首捻り", "head twisting throw", Throw},
	"makiotoshi":       {"巻き落とし", "twist down", Throw},
	"osakate":          {"大逆手", "backward belt throw", Throw},
	"sakatottari":      {"逆とったり", "arm bar throw counter", Throw},
	"shitatehineri":    {"下手捻り", "twisting underarm throw", Throw},
	"sotomuso":         {"外無双", "outer thigh propping twist down", Throw},
	"tokkurinage":      {"徳利投げ", "two-handed head twist down", Throw},
	"tottari":          {"とったり", "arm bar throw", Throw},
	"tsukiotoshi":      {"突き落とし", "thrust down", Throw},
	"uchimuso":         {"内無双", "inner thigh propping twist down", Throw},
	"uwatehineri":      {"上手捻り", "twisting overarm throw", Throw},
	"zubuneri":         {"ずぶねり", "head pivot throw", Throw},
	"utchari":          {"うっちゃり", "backward pivot throw", Throw},
	"yobimodoshi":      {"呼び戻し", "pulling body slam", Throw},
	"hatakikomi":       {"叩き込み", "slap down", Pull},
	"hikiotoshi":       {"引き落とし", "hand pull down", Pull},
	"hikkake":          {"引っ掛け", "arm grabbing force out", Pull},
	"katasukashi":      {"肩透かし", "under-shoulder swing down", Pull},
	"okurihikiotoshi":  {"送り引き落とし", "rear pull down", Pull},
	"sokubiotoshi":     {"素首落とし", "head chop down", Pull},
	"ushiromotare":     {"後ろもたれ", "backward lean out", Other},
	"isamiashi":        {"勇み足", "forward step out (non-technique)", Other},
	"koshikudake":      {"腰砕け", "inadvertent collapse (non-technique)", Other},
	"tsukite":          {"つき手", "hand down (non-technique)", Other},
	"tsukihiza":        {"つきひざ", "knee down (non-technique)", Other},
	"fumidashi":        {"踏み出し", "rear step out (non-technique)", Other},
	"hansoku":          {"反則", "disqualification", Other},
	"fusen":            {"不戦", "win/loss by default", NoTechnique},
	"dashinage":        {"出し投げ", "pulling throw (old)", Throw},
	"yobikaeshi":       {"呼び返し", "pulling body slam (old name of yobimodoshi)", Throw},
	"hikiwake":         {"引き分け", "draw (old era)", NoTechnique},
	"itamiwake":        {"痛み分け", "draw by injury (old era)", NoTechnique},
}

var kimariteByKanji = func() map[string]Kimarite {
	m := make(map[string]Kimarite, len(kimariteTable))
	for k, info := range kimariteTable {
		m[info.Kanji] = k
	}
	return m
}()

// Info returns the table entry for k.
func (k Kimarite) Info() (KimariteInfo, bool) {
	info, ok := kimariteTable[k]
	return info, ok
}

// Category returns k's style group. Unknown techniques count as Other
// (SPEC Appendix B).
func (k Kimarite) Category() KimariteCategory {
	if info, ok := kimariteTable[k]; ok {
		return info.Category
	}
	return Other
}

var romajiSeparators = strings.NewReplacer(" ", "", "-", "")

// NormalizeKimarite turns a romaji spelling into the canonical key: lower
// case, without spaces or hyphens ("Uwate-dashinage" → "uwatedashinage").
func NormalizeKimarite(s string) Kimarite {
	return Kimarite(romajiSeparators.Replace(strings.ToLower(strings.TrimSpace(s))))
}

// KimariteByKanji maps an exact Japanese technique name ("寄り切り") to its
// key. Source-specific cleanup, such as osada.us's prefixes, belongs in the
// adapter.
func KimariteByKanji(s string) (Kimarite, bool) {
	k, ok := kimariteByKanji[s]
	return k, ok
}
