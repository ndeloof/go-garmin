package garmin

import "iter"

// ListOptions selects one page of a start/limit-paginated collection.
type ListOptions struct {
	Start int // offset of the first item (0-based)
	Limit int // page size; 0 means the service default
}

func (o *ListOptions) startLimit(defaultLimit int) (int, int) {
	if o == nil {
		return 0, defaultLimit
	}
	limit := o.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	return o.Start, limit
}

// paged turns a page-fetching function into an iterator that walks the whole
// collection, stopping at the first short/empty page. Errors are yielded once
// and end the iteration.
func paged[T any](pageSize int, fetch func(start, limit int) ([]T, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		start := 0
		for {
			items, err := fetch(start, pageSize)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, it := range items {
				if !yield(it, nil) {
					return
				}
			}
			if len(items) < pageSize {
				return
			}
			start += len(items)
		}
	}
}

type dateRange struct{ start, end Date }

// chunkDateRange splits [start, end] (inclusive) into consecutive ranges of
// at most maxDays days each — some Garmin endpoints (e.g. daily steps) cap
// the queryable window at 28 days.
func chunkDateRange(start, end Date, maxDays int) []dateRange {
	if end.Before(start) {
		return nil
	}
	var out []dateRange
	for cur := start; !cur.After(end); {
		chunkEnd := cur.AddDays(maxDays - 1)
		if chunkEnd.After(end) {
			chunkEnd = end
		}
		out = append(out, dateRange{cur, chunkEnd})
		cur = chunkEnd.AddDays(1)
	}
	return out
}
