package testutil

import "github.com/serverkraken/flow/internal/domain"

// FakePaletteEntryReader returns a fixed entry list.
type FakePaletteEntryReader struct {
	Entries []domain.PaletteEntry
	Err     error
}

func (f *FakePaletteEntryReader) List() ([]domain.PaletteEntry, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]domain.PaletteEntry, len(f.Entries))
	copy(out, f.Entries)
	return out, nil
}

// FakePaletteStatsStore is an in-memory PaletteStatsStore. Stats is the
// canonical store; Load returns a copy, Save replaces it wholesale.
type FakePaletteStatsStore struct {
	Stats   domain.PaletteStats
	LoadErr error
	SaveErr error
}

func (f *FakePaletteStatsStore) Load() (domain.PaletteStats, error) {
	if f.LoadErr != nil {
		return domain.PaletteStats{}, f.LoadErr
	}
	out := domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{}}
	for k, v := range f.Stats.Actions {
		out.Actions[k] = v
	}
	return out, nil
}

func (f *FakePaletteStatsStore) Save(s domain.PaletteStats) error {
	if f.SaveErr != nil {
		return f.SaveErr
	}
	f.Stats = domain.PaletteStats{Actions: map[string]domain.PaletteActionStat{}}
	for k, v := range s.Actions {
		f.Stats.Actions[k] = v
	}
	return nil
}
