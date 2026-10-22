package exploration

// HasSelection reports whether the spec selects anything to run: a dimension, a metric, or a time
// grain. A spec with none of them has no output columns and cannot be executed or exported.
func (s *ExplorationSpec) HasSelection() bool {
	return s != nil && (len(s.Dimensions) > 0 || len(s.Metrics) > 0 || s.Time != nil)
}
