package http

import (
	"fmt"

	exploration "github.com/flidai/leapview/internal/analytics/exploration"
	saved "github.com/flidai/leapview/internal/analytics/exploration/saved"
	projectsignals "github.com/flidai/leapview/internal/project/ui/signals"
)

func savedCommandSpec(command projectsignals.SavedExplorationCommandSignal) (exploration.ExplorationSpec, saved.ExplorationSpecPayload, error) {
	if command.Spec == nil {
		return exploration.ExplorationSpec{}, saved.ExplorationSpecPayload{}, fmt.Errorf("%w: authored exploration spec is required", saved.ErrInvalid)
	}
	spec := *command.Spec
	// Browser signal merging can retain an empty optional time alias. Treat
	// only that exact value as omitted, without changing the incoming command.
	if spec.Time != nil && spec.Time.Alias != nil && *spec.Time.Alias == "" {
		timeSelection := *spec.Time
		timeSelection.Alias = nil
		spec.Time = &timeSelection
	}
	if err := exploration.ValidateShape(&spec); err != nil {
		return exploration.ExplorationSpec{}, saved.ExplorationSpecPayload{}, err
	}
	payload, err := saved.NewExplorationSpecPayload(spec)
	if err != nil {
		return exploration.ExplorationSpec{}, saved.ExplorationSpecPayload{}, err
	}
	return spec, payload, nil
}
