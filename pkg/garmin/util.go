package garmin

import "net/url"

// urlValues builds url.Values from alternating key/value pairs.
func urlValues(kv ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	return q
}
