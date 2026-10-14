package dev

import (
	"campus-resource-platform/backend/internal/m2"
	"context"
	"encoding/json"
	"errors"
	"os"
)

// M2Fixture is a development adapter, not production D-01/D-02.
type M2Fixture struct {
	Tokens   map[string]m2.Actor `json:"m2Tokens"`
	Policy   m2.Policy           `json:"m2Policy"`
	RoomData []m2.Room           `json:"m2Rooms"`
}

func LoadM2(path string) (*M2Fixture, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var f M2Fixture
	if e = json.Unmarshal(raw, &f); e != nil {
		return nil, e
	}
	if len(f.Tokens) == 0 || len(f.RoomData) == 0 {
		return nil, errors.New("M2 development fixture missing")
	}
	if f.Policy.Version == "" || f.Policy.MaxHours < 1 || f.Policy.MaxHours > 4 || f.Policy.AdvanceDays < 1 || f.Policy.ApprovalHours < 1 {
		return nil, errors.New("invalid M2 development policy; maxHours must be 1..4 for current M1 availability API")
	}
	return &f, nil
}
func (f *M2Fixture) Authenticate(_ context.Context, token string) (m2.Actor, error) {
	a, ok := f.Tokens[token]
	if !ok {
		return a, errors.New("unauthenticated")
	}
	return a, nil
}
func (f *M2Fixture) Rooms(context.Context) ([]m2.Room, error) { return f.RoomData, nil }
func (f *M2Fixture) Actors(context.Context) ([]m2.Actor, error) {
	out := []m2.Actor{}
	seen := map[string]bool{}
	for _, a := range f.Tokens {
		if !seen[a.ID] {
			out = append(out, a)
			seen[a.ID] = true
		}
	}
	return out, nil
}
