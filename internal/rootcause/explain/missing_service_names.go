package explain

// closestName is the one of names a very small number of edits away
// from missing: "payments" for "paymnts". Short names allow no edit, a
// name of ten letters or more two. It is "" when none is that close or
// two are equally close: a guess between them would be a guess.
func closestName(missing string, names []string) string {
	allowed := min(len(missing)/5, 2)
	best, bestDistance := "", allowed+1
	tied := false
	for _, name := range names {
		switch d := editDistance(missing, name); {
		case d < bestDistance:
			best, bestDistance, tied = name, d, false
		case d == bestDistance:
			tied = true
		}
	}
	if tied || bestDistance > allowed {
		return ""
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		row := make([]int, len(b)+1)
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			row[j] = min(previous[j]+1, row[j-1]+1, previous[j-1]+cost)
		}
		previous = row
	}
	return previous[len(b)]
}
