package testutil

import "github.com/serverkraken/flow/internal/domain"

// FakeFlowStateStore is an in-memory FlowStateStore. State is the
// canonical persisted value; NextScreen is the one-shot deep-link slot.
type FakeFlowStateStore struct {
	State      domain.FlowState
	NextScreen string
	LoadErr    error
	SaveErr    error
}

func (f *FakeFlowStateStore) Load() (domain.FlowState, error) {
	if f.LoadErr != nil {
		return domain.FlowState{}, f.LoadErr
	}
	return f.State, nil
}

func (f *FakeFlowStateStore) Save(s domain.FlowState) error {
	if f.SaveErr != nil {
		return f.SaveErr
	}
	f.State = s
	return nil
}

func (f *FakeFlowStateStore) ConsumeNextScreen() (string, error) {
	s := f.NextScreen
	f.NextScreen = ""
	return s, nil
}

func (f *FakeFlowStateStore) WriteNextScreen(screen string) error {
	f.NextScreen = screen
	return nil
}
