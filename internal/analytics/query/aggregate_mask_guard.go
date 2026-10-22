package query

import (
	"fmt"
	"strings"
)

// rejectMaskedAggregateInputs fails closed when an aggregate request groups or
// filters by a masked field. Grouping would return the raw values as group keys
// and a filter would act as a prefix/equality oracle over them, so the masked
// column's raw data would be recoverable even though it is never projected. This
// matches the treatment of masked metric inputs. Sorts can only name selected
// output columns, so a masked sort key is rejected through its dimension.
func (p *Planner) rejectMaskedAggregateInputs(request Request, resolved aggregateResolution) error {
	if len(resolved.Masks) == 0 {
		return nil
	}
	for _, dimension := range resolved.Dimensions {
		if resolved.Masks.matchesAny(p.maskCandidates(dimension.Name, "", resolved.Datasets)) {
			return fmt.Errorf("dimension %q depends on a masked field", dimension.Name)
		}
	}
	var walk func(Filter) error
	walk = func(filter Filter) error {
		check := func(field, dataset string) error {
			if field != "" && resolved.Masks.matchesAny(p.maskCandidates(field, dataset, resolved.Datasets)) {
				return fmt.Errorf("filter field %q depends on a masked field", field)
			}
			return nil
		}
		if err := check(filter.Field, filter.Dataset); err != nil {
			return err
		}
		if filter.Spatial != nil {
			if err := check(filter.Spatial.LatitudeField, filter.Spatial.Dataset); err != nil {
				return err
			}
			if err := check(filter.Spatial.LongitudeField, filter.Spatial.Dataset); err != nil {
				return err
			}
		}
		for _, group := range filter.Groups {
			for _, child := range group.Filters {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, filter := range request.Filters {
		if err := walk(filter); err != nil {
			return err
		}
	}
	return nil
}

// maskCandidates lists every name a mask policy may use for a request field:
// the reference itself and the physical dataset.field it resolves to in each
// participating dataset.
func (p *Planner) maskCandidates(ref, filterDataset string, datasets []string) []string {
	candidates := []string{ref}
	if p == nil || p.compiled == nil {
		return candidates
	}
	if _, semantic := p.compiled.SemanticDimension(ref); semantic {
		for _, dataset := range datasets {
			if filterDataset != "" && filterDataset != dataset {
				continue
			}
			if binding, ok := p.compiled.DimensionBinding(ref, dataset); ok {
				candidates = append(candidates, binding.Physical.Field, binding.Physical.Table+"."+binding.Physical.Name)
			}
		}
		return candidates
	}
	if physical, err := p.resolveDimension(ref); err == nil {
		candidates = append(candidates, physical.Field, physical.Table+"."+physical.Name)
	}
	return candidates
}

func (m columnMaskSet) matchesAny(candidates []string) bool {
	for _, candidate := range candidates {
		if _, ok := m[strings.ToLower(strings.TrimSpace(candidate))]; ok {
			return true
		}
	}
	return false
}
