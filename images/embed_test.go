package images

import "testing"

// A Dockerfile without the label would make doctor rebuild the image on every start.
func TestEveryImageDeclaresARevision(t *testing.T) {
	for _, dir := range []string{"node", "terminal"} {
		if Rev(dir) == "" {
			t.Errorf("images/%s/Dockerfile has no LABEL %s", dir, RevLabel)
		}
	}
}
