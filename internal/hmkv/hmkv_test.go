package hmkv

import "testing"

func TestApplyTitleSelection(t *testing.T) {
	titles := []TitleInfo{
		{Index: 0, FileName: "short.mkv", Length: "0:05:00", FileSizeBytes: 100},
		{Index: 1, FileName: "feature.mkv", Length: "1:45:12", FileSizeBytes: 800},
		{Index: 2, FileName: "extra.mkv", Length: "0:10:00", FileSizeBytes: 200},
	}

	t.Run("empty selects longest", func(t *testing.T) {
		got, err := applyTitleSelection(titles, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Index != 1 {
			t.Fatalf("got %+v, want title index 1", got)
		}
	})

	t.Run("longest keyword", func(t *testing.T) {
		got, err := applyTitleSelection(titles, "longest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Index != 1 {
			t.Fatalf("got %+v, want title index 1", got)
		}
	})

	t.Run("all", func(t *testing.T) {
		got, err := applyTitleSelection(titles, "all")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d titles, want 3", len(got))
		}
	})

	t.Run("explicit ids", func(t *testing.T) {
		got, err := applyTitleSelection(titles, "0,2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].Index != 0 || got[1].Index != 2 {
			t.Fatalf("got %+v, want indexes 0 and 2", got)
		}
	})

	t.Run("explicit ids preserve selection order", func(t *testing.T) {
		got, err := applyTitleSelection(titles, "2,0,1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0].Index != 2 || got[1].Index != 0 || got[2].Index != 1 {
			t.Fatalf("got %+v, want indexes 2, 0, 1", got)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		_, err := applyTitleSelection(titles, "nope")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestValidateTitleMode(t *testing.T) {
	tests := []struct {
		mode    string
		wantErr bool
	}{
		{"", false},
		{"longest", false},
		{"all", false},
		{"0,1,2", false},
		{"nope", true},
		{"-1", true},
	}
	for _, tt := range tests {
		err := ValidateTitleMode(tt.mode)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateTitleMode(%q) error = %v, wantErr = %v", tt.mode, err, tt.wantErr)
		}
	}
}
