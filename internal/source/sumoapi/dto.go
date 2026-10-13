package sumoapi

import "time"

// The DTOs mirror sumo-api.com's JSON (SPEC Appendix A.1). Field names follow
// the API; note rikishiID (banzuke) vs rikishiId (everywhere else).

type bashoDTO struct {
	Date          string     `json:"date"` // "" in the zero object of an unknown tournament
	Location      string     `json:"location"`
	StartDate     time.Time  `json:"startDate"`
	EndDate       time.Time  `json:"endDate"`
	Yusho         []awardDTO `json:"yusho"`
	SpecialPrizes []awardDTO `json:"specialPrizes"`
}

type awardDTO struct {
	Type      string `json:"type"`
	RikishiID int    `json:"rikishiId"`
}

type banzukeDTO struct {
	East []banzukeEntryDTO `json:"east"`
	West []banzukeEntryDTO `json:"west"`
}

type banzukeEntryDTO struct {
	RikishiID int         `json:"rikishiID"`
	ShikonaEn string      `json:"shikonaEn"`
	ShikonaJp string      `json:"shikonaJp"`
	Rank      string      `json:"rank"`
	Record    []recordDTO `json:"record"`
}

type recordDTO struct {
	Result     string `json:"result"`
	OpponentID int    `json:"opponentID"`
	Kimarite   string `json:"kimarite"`
}

type torikumiDTO struct {
	Torikumi []boutDTO `json:"torikumi"`
}

type boutDTO struct {
	Day         int    `json:"day"`
	MatchNo     int    `json:"matchNo"`
	EastID      int    `json:"eastId"`
	EastShikona string `json:"eastShikona"`
	EastRank    string `json:"eastRank"`
	WestID      int    `json:"westId"`
	WestShikona string `json:"westShikona"`
	WestRank    string `json:"westRank"`
	Kimarite    string `json:"kimarite"`
	WinnerID    int    `json:"winnerId"`
}

type rikishiPageDTO struct {
	Total   int          `json:"total"`
	Records []rikishiDTO `json:"records"`
}

type rikishiDTO struct {
	ID        int       `json:"id"`
	SumoDBID  int       `json:"sumodbId"`
	NSKID     int       `json:"nskId"`
	ShikonaEn string    `json:"shikonaEn"`
	ShikonaJp string    `json:"shikonaJp"`
	Heya      string    `json:"heya"`
	Shusshin  string    `json:"shusshin"`
	BirthDate time.Time `json:"birthDate"`
	Height    float64   `json:"height"`
	Weight    float64   `json:"weight"`
	Debut     string    `json:"debut"`
}
