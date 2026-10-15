package service

// Credit is one data source and how basho uses it (`basho credits`, the
// help screen).
type Credit struct{ Source, About string }

// Credits are the data sources, in the order basho asks them.
var Credits = []Credit{
	{"sumo-api.com", "History, profiles and statistics since 1960, run by a volunteer. " +
		"basho keeps to its request for responsible use: at most one request per second, and finished tournaments ship inside basho."},
	{"osada.us", "Live results of the current tournament, scraped from the Japan Sumo Association's official pages " +
		"and run by one person on a best-effort basis. Its usage policy allows search and AI input, not AI training; " +
		"basho shows the source and time of its data."},
}

// CreditsNote follows the sources.
const CreditsNote = "The embedded snapshot of finished tournaments is built from sumo-api.com. " +
	"Ratings are basho's own Elo-style estimate from past results, not an official ranking and not betting advice."
