package main

import (
	"context"

	"campus-resource-platform/backend/internal/common/identity"
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/internal/m1"
	"campus-resource-platform/backend/internal/m2"
)

type m2Directory struct {
	identity *identity.Store
	fixture  *dev.M2Fixture
}

func (d *m2Directory) Authenticate(ctx context.Context, token string) (m2.Actor, error) {
	session, err := d.identity.Authenticate(ctx, token)
	if err != nil {
		return m2.Actor{}, err
	}
	return m2Actor(session.Principal), nil
}

func (d *m2Directory) Actors(ctx context.Context) ([]m2.Actor, error) {
	principals, err := d.identity.Principals(ctx)
	if err != nil {
		return nil, err
	}
	actors := make([]m2.Actor, 0, len(principals))
	for _, principal := range principals {
		actors = append(actors, m2Actor(principal))
	}
	return actors, nil
}

func (d *m2Directory) Rooms(ctx context.Context) ([]m2.Room, error) {
	return d.fixture.Rooms(ctx)
}

func m2Actor(principal identity.Principal) m2.Actor {
	return m2.Actor{
		ID:          principal.ID,
		Department:  principal.Department,
		Permissions: principal.Permissions,
		Areas:       principal.Areas,
		ContactRefs: principal.ContactRefs,
	}
}

func m1Authenticate(store *identity.Store, fixture *dev.Fixture) m1.Authenticate {
	return func(ctx context.Context, token string) (m1.Actor, error) {
		session, err := store.Authenticate(ctx, token)
		if err == nil {
			role := ""
			if len(session.Principal.Roles) > 0 {
				role = session.Principal.Roles[0]
			}
			return m1.Actor{
				ID:         session.Principal.ID,
				Department: session.Principal.Department,
				Role:       role,
			}, nil
		}
		service, serviceErr := fixture.Auth(ctx, token)
		if serviceErr != nil || service.Service == "" {
			return m1.Actor{}, err
		}
		return service, nil
	}
}
