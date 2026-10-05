package routes

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"schej.it/server/models"
)

func TestIcsContentDisposition(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Team dinner", `attachment; filename="Team dinner.ics"; filename*=UTF-8''Team%20dinner.ics`},
		{"Grillen bei Jürgen", `attachment; filename="Grillen bei Jrgen.ics"; filename*=UTF-8''Grillen%20bei%20J%C3%BCrgen.ics`},
		{`"; evil/../\name`, `attachment; filename="evil..name.ics"; filename*=UTF-8''evil..name.ics`},
		{"Grillen – bei  Jürgen", `attachment; filename="Grillen bei Jrgen.ics"; filename*=UTF-8''Grillen%20bei%20J%C3%BCrgen.ics`},
		{"🎉🎉", `attachment; filename="event.ics"; filename*=UTF-8''event.ics`},
		{"", `attachment; filename="event.ics"; filename*=UTF-8''event.ics`},
	}
	for _, tt := range tests {
		if got := icsContentDisposition(tt.name); got != tt.want {
			t.Errorf("icsContentDisposition(%q)\n got %s\nwant %s", tt.name, got, tt.want)
		}
	}
}

func TestIcsContentDisposition_TruncatesOnRuneBoundary(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "ü"
	}
	got := icsContentDisposition(long)
	want := "filename*=UTF-8''"
	for i := 0; i < 80; i++ {
		want += "%C3%BC"
	}
	if got != `attachment; filename="event.ics"; `+want+".ics" {
		t.Errorf("got %s", got)
	}
}

func TestScheduledEventUID(t *testing.T) {
	event := &models.Event{Id: primitive.NewObjectID()}

	t.Setenv("BASE_URL", "https://time.example.com/")
	if got, want := scheduledEventUID(event), "timeful-"+event.Id.Hex()+"@time.example.com"; got != want {
		t.Errorf("uid = %q, want %q", got, want)
	}
	if got, want := scheduledEventFileName(event), "timeful-"+event.Id.Hex()+".ics"; got != want {
		t.Errorf("file name = %q, want %q", got, want)
	}
}
