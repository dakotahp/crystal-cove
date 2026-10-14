package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthNamesTheBuild(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.Handler(AuthConfig{StaticToken: "secret"}))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "ok\n") || !strings.Contains(string(body), BuildInfo()) {
		t.Errorf("body = %q, want ok then %q", body, BuildInfo())
	}
}

func TestBuildInfoNamesVersionAndCommit(t *testing.T) {
	old := Commit
	t.Cleanup(func() { Commit = old })
	Commit = "abc1234"

	if got, want := BuildInfo(), "crystal-cove "+Version+" (commit abc1234)"; got != want {
		t.Errorf("BuildInfo() = %q, want %q", got, want)
	}
}
