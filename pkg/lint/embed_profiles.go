package lint

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/workato-devs/wk-lint-beta/profiles"
)

func loadEmbeddedProfiles() (map[string]*ProfileDef, error) {
	result := make(map[string]*ProfileDef)

	entries, err := profiles.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("reading embedded profiles: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := profiles.FS.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading embedded profile %s: %w", entry.Name(), err)
		}

		var prof ProfileDef
		if err := json.Unmarshal(data, &prof); err != nil {
			return nil, fmt.Errorf("parsing embedded profile %s: %w", entry.Name(), err)
		}

		expectedName := strings.TrimSuffix(entry.Name(), ".json")
		if prof.Name != expectedName {
			return nil, fmt.Errorf("embedded profile %s: name field %q must match filename %q",
				entry.Name(), prof.Name, expectedName)
		}

		result[prof.Name] = &prof
	}

	return result, nil
}
