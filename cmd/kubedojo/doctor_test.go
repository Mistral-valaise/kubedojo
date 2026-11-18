package main

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
)

// needsCatalog has a cluster track, a container track and a theory track.
func needsCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	const head = "exam: {durationMinutes: 10, passPercent: 50, questions: 1}\ndomains: [{id: d, title: D, weight: 100}]\n"
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	cat, err := catalog.Load(fstest.MapFS{
		"cluster/track.yaml":   file("title: C\nkubernetesVersion: v1.37\n" + head + "environments: {base: {}, ops: {workers: 1}}\n"),
		"cluster/lessons/d.md": file("# D"),
		"host/track.yaml":      file("title: H\nkind: skills\n" + head + "environments: {base: {type: container}}\n"),
		"host/lessons/d.md":    file("# D"),
		"quiz/track.yaml":      file("title: Q\nkind: theory\n" + head),
		"quiz/lessons/d.md":    file("# D"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// A track without a cluster must run on a machine without kind, and a track
// without a Linux host must not make anybody build its image.
func TestNeedsFollowTheSelectedTracks(t *testing.T) {
	cat := needsCatalog(t)
	drv := &sandbox.Kind{NodeImage: "node", TerminalImage: "terminal", LinuxImage: "linux"}
	for name, c := range map[string]struct {
		tracks []string
		want   needs
		images []string // build contexts, in the order they are checked
	}{
		"a cluster track":   {[]string{"cluster"}, needs{cluster: true}, []string{"node", "terminal"}},
		"a container track": {[]string{"host"}, needs{container: true}, []string{"terminal", "linux"}},
		"a theory track":    {[]string{"quiz"}, needs{}, nil},
		"cluster + theory":  {[]string{"cluster", "quiz"}, needs{cluster: true}, []string{"node", "terminal"}},
		"all tracks":        {[]string{"cluster", "host", "quiz"}, everything, []string{"node", "terminal", "linux"}},
		"no track":          {nil, needs{}, nil},
	} {
		var tracks []*catalog.Track
		for _, id := range c.tracks {
			tracks = append(tracks, cat.Tracks[id])
		}
		got := needsOf(tracks)
		if got != c.want {
			t.Errorf("%s: needs = %+v, want %+v", name, got, c.want)
		}
		var dirs []string
		for _, img := range imagesFor(drv, got) {
			// Every image is checked under the tag the driver will run.
			if img[0] != img[1] {
				t.Errorf("%s: context %s is checked as image %q", name, img[1], img[0])
			}
			dirs = append(dirs, img[1])
		}
		if !slices.Equal(dirs, c.images) {
			t.Errorf("%s: images = %v, want %v", name, dirs, c.images)
		}
	}
}

func TestTrackNeeds(t *testing.T) {
	cat := needsCatalog(t)
	for id, want := range map[string][2]bool{"cluster": {true, false}, "host": {false, true}, "quiz": {false, false}} {
		tr := cat.Tracks[id]
		if got := [2]bool{tr.NeedsCluster(), tr.NeedsContainer()}; got != want {
			t.Errorf("%s: NeedsCluster, NeedsContainer = %v, want %v", id, got, want)
		}
	}
}
