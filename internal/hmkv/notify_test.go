package hmkv

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendNtfyPostsTitleAndBody(t *testing.T) {
	var gotMethod, gotPath, gotTitle, gotContentType string
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		gotContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		gotBody = string(body)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := SendNtfy(server.URL, "my-secret-topic", "HandyMKV test", "Ready for next disc")
	if err != nil {
		t.Fatalf("SendNtfy returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/my-secret-topic" {
		t.Errorf("path = %q, want /my-secret-topic", gotPath)
	}
	if gotTitle != "HandyMKV test" {
		t.Errorf("Title header = %q, want HandyMKV test", gotTitle)
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", gotContentType)
	}
	if gotBody != "Ready for next disc" {
		t.Errorf("body = %q, want Ready for next disc", gotBody)
	}
}

func TestSendNtfyEmptyTopicNoOp(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := SendNtfy(server.URL, "  ", "title", "message"); err != nil {
		t.Fatalf("SendNtfy returned error: %v", err)
	}
	if called {
		t.Fatal("expected no HTTP request for empty topic")
	}
}

func TestSendNtfyTrimsServerTrailingSlash(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := SendNtfy(server.URL+"/", "topic", "", "hello")
	if err != nil {
		t.Fatalf("SendNtfy returned error: %v", err)
	}
	if gotPath != "/topic" {
		t.Errorf("path = %q, want /topic", gotPath)
	}
}

func TestSendNtfyNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "topic not found", http.StatusNotFound)
	}))
	defer server.Close()

	err := SendNtfy(server.URL, "missing-topic", "title", "message")
	if err == nil {
		t.Fatal("expected error for non-success status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want status 404 mentioned", err.Error())
	}
}

func TestMaybeNotifySkipsWhenTopicUnset(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &handyMKVConfig{
		NtfyServer: server.URL,
	}
	maybeNotify(config, "title", "message")
	if called {
		t.Fatal("expected no HTTP request when ntfy topic is unset")
	}
}

func TestMovieLabelForNotification(t *testing.T) {
	t.Run("cli movie name with year", func(t *testing.T) {
		got := movieLabelForNotification(nil, nil, "The Matrix (1999)", "")
		if got != "The Matrix (1999)" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("cli movie name and year flags", func(t *testing.T) {
		got := movieLabelForNotification(nil, nil, "The Matrix", "1999")
		if got != "The Matrix (1999)" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("library path folder name", func(t *testing.T) {
		got := movieLabelForNotification([]string{`D:\movies\Blade Runner (1982)\Blade Runner (1982).mkv`}, nil, "", "")
		if got != "Blade Runner (1982)" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("disc metadata fallback", func(t *testing.T) {
		titles := []TitleInfo{{
			FileName:  "BLADE_RUNNER",
			DiscTitle: "Blade Runner (1982)",
		}}
		got := movieLabelForNotification(nil, titles, "", "")
		if !strings.Contains(got, "Blade Runner") {
			t.Errorf("got %q", got)
		}
	})
}

func TestNotifyDiscFreeUsesConfig(t *testing.T) {
	var gotTitle, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &handyMKVConfig{
		NtfyTopic:  "handymkv-test",
		NtfyServer: server.URL,
	}

	notifyDiscFree(config, nil, ExecOptions{MediaName: "Inception (2010)"}, nil)

	if gotTitle != "HandyMKV — Disc free" {
		t.Errorf("Title header = %q, want HandyMKV — Disc free", gotTitle)
	}
	if !strings.Contains(gotBody, "DISC FREE — insert next disc now.") {
		t.Errorf("body = %q, want disc-free message", gotBody)
	}
	if !strings.Contains(gotBody, "Inception (2010)") {
		t.Errorf("body = %q, want movie name", gotBody)
	}
}

func TestMediaLabelForNotificationTV(t *testing.T) {
	opts := ExecOptions{TVMode: true, MediaName: "Star Trek", MediaYear: "1966", Season: 1, StartEpisode: 1}
	meta := &tvSeriesMeta{Series: "Star Trek", Year: "1966", Season: 1, StartEpisode: 1}
	got := mediaLabelForNotification(nil, make([]TitleInfo, 4), opts, meta)
	if got != "Star Trek (1966) S01E01–E04" {
		t.Fatalf("got %q", got)
	}
}
