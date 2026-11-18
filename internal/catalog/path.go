package catalog

import (
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

// Path is the recommended order through the tracks, from content/path.yaml.
// It only recommends: no track is locked behind another.
type Path struct {
	ID         string  `yaml:"id" json:"id"`
	Title      string  `yaml:"title" json:"title"`
	Summary    string  `yaml:"summary" json:"summary"`
	URL        string  `yaml:"url" json:"url"`
	Disclaimer string  `yaml:"disclaimer" json:"disclaimer"`
	Steps      []*Step `yaml:"steps" json:"steps"`
}

// Step roles: a foundation prepares for the goal, a required step is part of it.
const (
	RoleFoundation = "foundation"
	RoleRequired   = "required"
)

type Step struct {
	Track string `yaml:"track" json:"track"`
	Role  string `yaml:"role" json:"role"`
	Why   string `yaml:"why" json:"why"`
}

// loadPath reads path.yaml. A catalog without one has no path.
func loadPath(fsys fs.FS, tracks map[string]*Track) (*Path, []error) {
	raw, err := fs.ReadFile(fsys, "path.yaml")
	if err != nil {
		return nil, nil
	}
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf("path.yaml: "+format, a...)) }
	p := &Path{}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(p); err != nil {
		return nil, []error{fmt.Errorf("path.yaml: %w", err)}
	}
	if p.ID == "" || p.Title == "" {
		fail("id and title are required")
	}
	seen := map[string]bool{}
	required := 0
	for _, s := range p.Steps {
		if tracks[s.Track] == nil {
			fail("unknown track %q", s.Track)
		}
		if seen[s.Track] {
			fail("track %q appears twice", s.Track)
		}
		seen[s.Track] = true
		switch s.Role {
		case RoleRequired:
			required++
		case RoleFoundation:
		default:
			fail("track %q: role must be %s or %s", s.Track, RoleFoundation, RoleRequired)
		}
	}
	if required == 0 {
		fail("at least one step with role %s is needed", RoleRequired)
	}
	return p, errs
}
