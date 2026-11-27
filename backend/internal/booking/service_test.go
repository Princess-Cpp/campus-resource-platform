package booking

import (
	"testing"
	"time"
)

func TestIntervalUsesWholeHoursAndMaximum(t *testing.T) {
	s := &Service{Zone: time.FixedZone("CST", 8*3600)}
	start := time.Date(2026, 10, 11, 9, 0, 0, 0, s.Zone)
	if err := s.interval(start, start.Add(4*time.Hour), 4); err != nil {
		t.Fatal(err)
	}
	for _, end := range []time.Time{start.Add(5 * time.Hour), start.Add(90 * time.Minute)} {
		if err := s.interval(start, end, 4); err == nil {
			t.Fatalf("expected invalid interval %s", end)
		}
	}
}
func TestCodeChangesByRoomAndDay(t *testing.T) {
	s := &Service{CodeSecret: []byte("01234567890123456789012345678901"), Zone: time.FixedZone("CST", 8*3600)}
	d := time.Date(2026, 10, 11, 0, 0, 0, 0, s.Zone)
	a, b := s.Code("room-1", d), s.Code("room-1", d.Add(24*time.Hour))
	if a == b || a != s.Code("room-1", d) {
		t.Fatal("daily code is not deterministic and scoped")
	}
}
func TestRoomStateRejectsContradictoryCatalog(t *testing.T) {
	for _, tc := range []struct {
		room Room
		want string
	}{
		{Room{Active: true}, "ACTIVE"},
		{Room{Active: false}, "UNKNOWN"},
		{Room{Active: false, Status: "DISABLED"}, "DISABLED"},
		{Room{Active: false, Status: "RETIRED"}, "RETIRED"},
		{Room{Active: true, Status: "DISABLED"}, "UNKNOWN"},
		{Room{Active: false, Status: "ACTIVE"}, "UNKNOWN"},
	} {
		if got := tc.room.State(); got != tc.want {
			t.Fatalf("state=%q, want %q for %+v", got, tc.want, tc.room)
		}
	}
}
