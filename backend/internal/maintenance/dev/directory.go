package dev

import (
	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
	"campus-resource-platform/backend/internal/maintenance/domain"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
)

// StaffBinding is local development policy for explicitly provisioned user IDs.
// No school-number pattern, registration input or browser field grants roles.
type StaffBinding struct {
	UserID      string   `json:"userId"`
	Permissions []string `json:"permissions"`
	Areas       []string `json:"areas"`
}
type Directory struct {
	Store            *identity.Store
	StudentReporting bool
	Staff            []StaffBinding
}

func LoadStaff(path string) ([]StaffBinding, error) {
	if path == "" {
		return nil, nil
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var staff []StaffBinding
	if e = json.Unmarshal(raw, &staff); e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	for _, b := range staff {
		if b.UserID == "" || len(b.UserID) > 64 || seen[b.UserID] || len(b.Areas) == 0 || len(b.Permissions) == 0 {
			return nil, fmt.Errorf("invalid M3 development staff binding")
		}
		seen[b.UserID] = true
		for _, p := range b.Permissions {
			if p != "workorder.dispatch" && p != "workorder.repair" && p != "workorder.accept" {
				return nil, fmt.Errorf("unsupported M3 development permission")
			}
		}
		for _, a := range b.Areas {
			if a == "" || len(a) > 128 {
				return nil, fmt.Errorf("invalid M3 development area")
			}
		}
	}
	return staff, nil
}
func (d *Directory) actor(p identity.Principal) domain.Actor {
	a := domain.Actor{ID: p.ID, Enabled: true, Department: p.Department, Permissions: append([]string{}, p.Permissions...), Areas: append([]string{}, p.Areas...)}
	// Current base role remains mandatory, so revocation is not bypassed by a binding.
	if slices.Contains(p.Roles, "student") {
		if d.StudentReporting && !a.Can("workorder.report") {
			a.Permissions = append(a.Permissions, "workorder.report")
		}
		for _, b := range d.Staff {
			if b.UserID == p.ID {
				a.Permissions = append(a.Permissions, b.Permissions...)
				a.Areas = append(a.Areas, b.Areas...)
			}
		}
	}
	return a
}
func (d *Directory) Authenticate(ctx context.Context, r *http.Request) (domain.Actor, error) {
	s, e := d.Store.AuthenticateRequest(ctx, r)
	if e != nil {
		return domain.Actor{}, e
	}
	return d.actor(s.Principal), nil
}
func (d *Directory) Workers(ctx context.Context) ([]maintenance.Worker, error) {
	principals, e := d.Store.Principals(ctx)
	if e != nil {
		return nil, e
	}
	out := []maintenance.Worker{}
	for _, p := range principals {
		a := d.actor(p)
		if a.Can("workorder.repair") {
			out = append(out, maintenance.Worker{ID: p.ID, Name: p.Name, Actor: a})
		}
	}
	return out, nil
}
