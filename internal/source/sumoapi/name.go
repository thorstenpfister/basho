package sumoapi

import "strings"

// cleanShikona turns sumo-api's kanji name into the display form (SPEC §5.6):
// "四股名　名" keeps only the ring name before the full-width space, and a
// bracketed reading such as "伊波(いなみ)" is dropped.
func cleanShikona(jp string) string {
	name, _, _ := strings.Cut(jp, "　")
	if i := strings.IndexAny(name, "(（"); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

// cleanShikonaEn does the same for the romaji name: sumo-api gives retired
// wrestlers' as "Hakuho Sho", ring name and given name.
func cleanShikonaEn(en string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(en), " ")
	return name
}
