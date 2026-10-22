package main

import (
	"context"

	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/classroom"
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/internal/identity"
)

type classroomDirectory struct {
	identity *identity.Store
	fixture  *dev.ClassroomFixture
}

func (d *classroomDirectory) Authenticate(ctx context.Context, token string) (classroom.Actor, error) {
	session, err := d.identity.Authenticate(ctx, token)
	if err != nil {
		return classroom.Actor{}, err
	}
	return classroomActor(session.Principal), nil
}

func (d *classroomDirectory) Actors(ctx context.Context) ([]classroom.Actor, error) {
	principals, err := d.identity.Principals(ctx)
	if err != nil {
		return nil, err
	}
	actors := make([]classroom.Actor, 0, len(principals))
	for _, principal := range principals {
		actors = append(actors, classroomActor(principal))
	}
	return actors, nil
}

func (d *classroomDirectory) Rooms(ctx context.Context) ([]classroom.Room, error) {
	return d.fixture.Rooms(ctx)
}

func classroomActor(principal identity.Principal) classroom.Actor {
	return classroom.Actor{
		ID:          principal.ID,
		Department:  principal.Department,
		Permissions: principal.Permissions,
		Areas:       principal.Areas,
		ContactRefs: principal.ContactRefs,
	}
}

func bookingAuthenticate(store *identity.Store, fixture *dev.Fixture) booking.Authenticate {
	return func(ctx context.Context, token string) (booking.Actor, error) {
		session, err := store.Authenticate(ctx, token)
		if err == nil {
			role := ""
			if len(session.Principal.Roles) > 0 {
				role = session.Principal.Roles[0]
			}
			return booking.Actor{
				ID:          session.Principal.ID,
				Department:  session.Principal.Department,
				Role:        role,
				Permissions: session.Principal.Permissions,
			}, nil
		}
		service, serviceErr := fixture.Auth(ctx, token)
		if serviceErr != nil || service.Service == "" {
			return booking.Actor{}, err
		}
		return service, nil
	}
}
