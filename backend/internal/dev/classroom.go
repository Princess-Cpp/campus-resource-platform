package dev

import (
	"campus-resource-platform/backend/internal/classroom"
	"context"
	"encoding/json"
	"errors"
	"os"
)

// ClassroomFixture is a development adapter, not production D-01/D-02.
type ClassroomFixture struct {
	Tokens   map[string]classroom.Actor `json:"m2Tokens"`
	Policy   classroom.Policy           `json:"m2Policy"`
	RoomData []classroom.Room           `json:"m2Rooms"`
}

func LoadClassroom(path string) (*ClassroomFixture, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var f ClassroomFixture
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
func (f *ClassroomFixture) Authenticate(_ context.Context, token string) (classroom.Actor, error) {
	a, ok := f.Tokens[token]
	if !ok {
		return a, errors.New("unauthenticated")
	}
	return a, nil
}
func (f *ClassroomFixture) Rooms(context.Context) ([]classroom.Room, error) { return f.RoomData, nil }
func (f *ClassroomFixture) Actors(context.Context) ([]classroom.Actor, error) {
	out := []classroom.Actor{}
	seen := map[string]bool{}
	for _, a := range f.Tokens {
		if !seen[a.ID] {
			out = append(out, a)
			seen[a.ID] = true
		}
	}
	return out, nil
}
