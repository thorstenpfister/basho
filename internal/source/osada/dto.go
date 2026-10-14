package osada

// DTOs of osada.us's JSON (SPEC Appendix A.2): only the fields basho uses.

type torikumiDTO struct {
	BashoName         string   `json:"bashoName"`
	Year              string   `json:"year"`
	UpdatedAt         string   `json:"updatedAt"`
	ResultUpdatedAt   string   `json:"resultUpdatedAt"`
	ScheduleUpdatedAt string   `json:"scheduleUpdatedAt"`
	ResultDays        []dayDTO `json:"resultDays"`
	ScheduleDays      []dayDTO `json:"scheduleDays"`
}

type dayDTO struct {
	Day      int    `json:"day"`
	ISODate  string `json:"isoDate"`
	PathDate string `json:"pathDate"` // YYYYMMDD; its month names the tournament
	Status   string `json:"status"`   // "published"
	Data     *struct {
		Makuuchi *divisionDTO `json:"makuuchi"`
		Juryo    *divisionDTO `json:"juryo"`
	} `json:"data"`
}

type divisionDTO struct {
	Matches   []matchDTO    `json:"matches"`
	Absentees []absenteeDTO `json:"absentees"`
}

type matchDTO struct {
	BoutNo         int     `json:"boutNo"`
	EastName       string  `json:"eastName"`
	EastRank       string  `json:"eastRank"`
	EastProfileURL string  `json:"eastProfileUrl"`
	WestName       string  `json:"westName"`
	WestRank       string  `json:"westRank"`
	WestProfileURL string  `json:"westProfileUrl"`
	Kimarite       string  `json:"kimarite"`
	Winner         *string `json:"winner"` // "east", "west" or null
	IsPlayoff      bool    `json:"isPlayoff"`
}

type absenteeDTO struct {
	ID         int    `json:"id"`
	ProfileURL string `json:"profileUrl"`
}

type banzukeDTO struct {
	BashoName string       `json:"bashoName"`
	Year      string       `json:"year"`
	UpdatedAt string       `json:"updatedAt"`
	Makuuchi  []banzukeRow `json:"makuuchi"`
	Juryo     []banzukeRow `json:"juryo"`
}

type banzukeRow struct {
	Title string       `json:"title"` // "横綱", "前頭1"
	East  []rikishiDTO `json:"east"`
	West  []rikishiDTO `json:"west"`
}

type rikishiDTO struct {
	ID      int       `json:"id"` // the JSA ID
	Name    string    `json:"name"`
	Yomi    string    `json:"yomi"`
	Rank    string    `json:"rank"`
	Results []*string `json:"results"` // "win", "loss", "draw" (absent) or null
}
