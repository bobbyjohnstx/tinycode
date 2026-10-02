package acp

import (
	"context"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

type StoreAdapter struct {
	store *session.Store
}

func NewStoreAdapter(store *session.Store) *StoreAdapter {
	return &StoreAdapter{store: store}
}

func (a *StoreAdapter) Create(_ context.Context, input session.CreateInput) (*session.Info, error) {
	return a.store.Create(input)
}

func (a *StoreAdapter) Get(_ context.Context, id string) (*session.Info, error) {
	return a.store.Get(id)
}

func (a *StoreAdapter) List(_ context.Context, projectID string) ([]*session.Info, error) {
	infos, err := a.store.List(projectID, 100, 0)
	if err != nil {
		return nil, err
	}
	result := make([]*session.Info, len(infos))
	for i := range infos {
		result[i] = &infos[i]
	}
	return result, nil
}

func (a *StoreAdapter) Delete(_ context.Context, id string) error {
	return a.store.Delete(id)
}
