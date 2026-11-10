// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package name

import (
	"maps"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

// Tables contains the information from a "name" table.
type Tables map[string]*Table

// Choose select the table which best matches the given language preferences.
func (tt Tables) Choose(prefs ...language.Tag) (*Table, language.Confidence) {
	keys := slices.Collect(maps.Keys(tt))
	if len(keys) == 0 {
		return nil, language.No
	}

	pref := make(map[string]int)
	for key, t := range tt {
		numFilled := 10 * len(t.keys())
		if key == "en-US" {
			numFilled += 55
		} else if key == "en" || strings.HasPrefix(key, "en-") {
			numFilled += 5
		}
		pref[key] = numFilled
	}

	sort.Slice(keys, func(i, j int) bool {
		keyI := keys[i]
		prefI := pref[keyI]
		keyJ := keys[j]
		prefJ := pref[keyJ]
		if prefI != prefJ {
			return prefI > prefJ
		}
		return keyI < keyJ
	})

	tags := make([]language.Tag, len(keys))
	for i, key := range keys {
		tags[i] = language.MustParse(key)
	}
	matcher := language.NewMatcher(tags)

	_, index, confidence := matcher.Match(prefs...)
	return tt[keys[index]], confidence
}
