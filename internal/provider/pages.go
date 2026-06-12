package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodePages unmarshals the output of `gh api --paginate` /
// `glab api --paginate` into one slice. Both CLIs emit each page as a
// SEPARATE top-level JSON array, back to back ("[...][...]"), so a
// plain json.Unmarshal into a slice works only for single-page
// responses and fails on exactly the busy targets where pagination
// kicks in (diffsmith-kjk). Decoding page-by-page handles one page and
// many identically; empty input yields an empty slice.
func DecodePages[T any](data []byte) ([]T, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var all []T
	for page := 1; ; page++ {
		var items []T
		if err := dec.Decode(&items); err != nil {
			if err == io.EOF {
				return all, nil
			}
			return nil, fmt.Errorf("decode page %d: %w", page, err)
		}
		all = append(all, items...)
	}
}
