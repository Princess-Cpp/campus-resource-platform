package dev

import (
	"campus-resource-platform/backend/internal/booking"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
)

type Fixture struct {
	Tokens    map[string]booking.Actor `json:"tokens"`
	RoomsData []booking.Room           `json:"rooms"`
	Emails    map[string]string        `json:"emails"`
}

func Load(path string) (*Fixture, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var f Fixture
	e = json.Unmarshal(raw, &f)
	if e != nil {
		return nil, e
	}
	var services struct {
		Tokens map[string]struct {
			Service string `json:"service"`
		} `json:"tokens"`
	}
	if e = json.Unmarshal(raw, &services); e != nil {
		return nil, e
	}
	for token, v := range services.Tokens {
		a := f.Tokens[token]
		a.Service = v.Service
		f.Tokens[token] = a
	}
	if len(f.Tokens) == 0 || len(f.RoomsData) == 0 {
		return nil, errors.New("empty dev fixture")
	}
	return &f, nil
}
func (f *Fixture) Rooms(context.Context, *sql.Tx) ([]booking.Room, error) { return f.RoomsData, nil }
func (f *Fixture) Auth(_ context.Context, t string) (booking.Actor, error) {
	a, ok := f.Tokens[t]
	if !ok {
		return booking.Actor{}, errors.New("unauthenticated")
	}
	if a.Service == "" && a.Role == "student" && len(a.Permissions) == 0 {
		a.Permissions = []string{"booking.create"}
	}
	return a, nil
}
