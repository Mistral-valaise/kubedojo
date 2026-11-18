package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

func getJSON(t *testing.T, url string, into any) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatalf("%s: %v", url, err)
	}
	return res.StatusCode
}

func TestPath(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Path == nil {
		t.Fatal("the shipped content has no path.yaml")
	}
	srv := httptest.NewServer((&api.Server{Catalog: cat}).Handler())
	defer srv.Close()

	var path struct {
		ID, Title, Summary, URL, Disclaimer string
		Steps                               []struct{ Track, Role, Why string }
	}
	if code := getJSON(t, srv.URL+"/api/path", &path); code != 200 {
		t.Fatalf("GET /api/path = %d, want 200", code)
	}
	if path.ID != cat.Path.ID || path.Title == "" || path.URL == "" || path.Disclaimer == "" {
		t.Errorf("path = %+v, want id %q with title, url and disclaimer", path, cat.Path.ID)
	}
	if len(path.Steps) != len(cat.Path.Steps) {
		t.Fatalf("%d steps, want %d", len(path.Steps), len(cat.Path.Steps))
	}

	// The UI shows the cards in path order and words each track by its kind.
	var tracks []struct {
		ID, Kind     string
		Environments map[string]struct {
			Type *string `json:"type"`
		}
	}
	if code := getJSON(t, srv.URL+"/api/tracks", &tracks); code != 200 {
		t.Fatalf("GET /api/tracks = %d, want 200", code)
	}
	for i, s := range path.Steps {
		if s.Role != catalog.RoleFoundation && s.Role != catalog.RoleRequired {
			t.Errorf("step %s: role %q", s.Track, s.Role)
		}
		if i >= len(tracks) || tracks[i].ID != s.Track {
			t.Errorf("track %d is not step %q: tracks do not follow the path", i, s.Track)
		}
	}
	for _, tr := range tracks {
		if tr.Kind == "" {
			t.Errorf("track %s: no kind", tr.ID)
		}
		for name, e := range tr.Environments {
			if e.Type == nil {
				t.Errorf("track %s: environment %s has no type field", tr.ID, name)
			}
		}
	}
}

func TestPathMissing(t *testing.T) {
	srv := httptest.NewServer((&api.Server{Catalog: &catalog.Catalog{}}).Handler())
	defer srv.Close()
	var body struct{ Error string }
	if code := getJSON(t, srv.URL+"/api/path", &body); code != 404 || body.Error == "" {
		t.Errorf("GET /api/path without a path = %d %q, want 404 with an error", code, body.Error)
	}
}
