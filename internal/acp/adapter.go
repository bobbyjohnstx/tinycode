package acp

import (
	"context"
	"fmt"

	"github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/session"
)

type StoreAdapter struct {
	store *session.Store
	msgs  *session.MessageStore
}

func NewStoreAdapter(store *session.Store) *StoreAdapter {
	return &StoreAdapter{
		store: store,
		msgs:  session.NewMessageStore(store),
	}
}

func (a *StoreAdapter) Create(_ context.Context, input session.CreateInput) (*session.Info, error) {
	return a.store.Create(input)
}

func (a *StoreAdapter) Get(_ context.Context, id string) (*session.Info, error) {
	return a.store.Get(id)
}

func (a *StoreAdapter) List(_ context.Context, projectID string) ([]*session.Info, error) {
	infos, err := a.store.List(projectID, 100, 0, false)
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

func (a *StoreAdapter) UpdateModel(_ context.Context, sessionID string, model *session.ModelRef) error {
	return a.store.UpdateModel(sessionID, model)
}

func (a *StoreAdapter) UpdateAgent(_ context.Context, sessionID, agent string) error {
	return a.store.UpdateAgent(sessionID, agent)
}

func (a *StoreAdapter) ListMessages(_ context.Context, sessionID string) ([]session.Message, error) {
	return a.msgs.List(sessionID)
}

func (a *StoreAdapter) Fork(_ context.Context, parentID, title string) (*session.Info, error) {
	parent, err := a.store.Get(parentID)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = parent.Title + " (fork)"
	}
	forked, err := a.store.Create(session.CreateInput{
		ProjectID: parent.ProjectID,
		Directory: parent.Directory,
		Title:     title,
		Agent:     parent.Agent,
		Model:     parent.Model,
		ParentID:  parent.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("create fork: %w", err)
	}

	parentMsgs, err := a.msgs.List(parentID)
	if err != nil {
		_ = a.store.Delete(forked.ID)
		return nil, fmt.Errorf("list parent messages: %w", err)
	}
	for i := range parentMsgs {
		msg := parentMsgs[i]
		msg.SessionID = forked.ID
		newID, idErr := id.Ascending("message")
		if idErr != nil {
			_ = a.store.Delete(forked.ID)
			return nil, fmt.Errorf("allocate message id: %w", idErr)
		}
		msg.ID = newID
		if err := a.msgs.Append(&msg); err != nil {
			_ = a.store.Delete(forked.ID)
			return nil, fmt.Errorf("copy message: %w", err)
		}
	}
	return forked, nil
}
