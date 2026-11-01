package types_test

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// Shared contains each distinct library annotation once. Fixtures keep only
// references to those entries and their own user-program bindings. Both the
// original binding paths and TypeString values are retained without rewriting.
type inferenceArchive struct {
	Shared   [][2]string                         `json:"shared"`
	Fixtures map[string]inferenceArchiveFixture `json:"fixtures"`
}

type inferenceArchiveFixture struct {
	Shared   []int             `json:"shared"`
	Bindings map[string]string `json:"bindings"`
}

func compactInference(captured inferenceCapture) inferenceArchive {
	archive := inferenceArchive{Fixtures: map[string]inferenceArchiveFixture{}}
	shared := map[[2]string]int{}
	var names []string
	for name := range captured.Types {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		library := map[string]bool{}
		for _, key := range captured.Libraries[name] {
			library[key] = true
		}
		bindings := captured.Types[name]
		var keys []string
		for key := range bindings {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fixture := inferenceArchiveFixture{Bindings: map[string]string{}}
		for _, key := range keys {
			typ := bindings[key]
			if !library[key] {
				fixture.Bindings[key] = typ
				continue
			}
			entry := [2]string{key, typ}
			index, exists := shared[entry]
			if !exists {
				index = len(archive.Shared)
				shared[entry] = index
				archive.Shared = append(archive.Shared, entry)
			}
			fixture.Shared = append(fixture.Shared, index)
		}
		archive.Fixtures[name] = fixture
	}
	return archive
}

func (archive inferenceArchive) expand() (inferenceSnapshot, error) {
	snapshot := inferenceSnapshot{}
	for name, fixture := range archive.Fixtures {
		bindings := make(map[string]string, len(fixture.Bindings)+len(fixture.Shared))
		for key, typ := range fixture.Bindings {
			bindings[key] = typ
		}
		for _, index := range fixture.Shared {
			if index < 0 || index >= len(archive.Shared) {
				return nil, fmt.Errorf("%s: invalid shared annotation %d", name, index)
			}
			entry := archive.Shared[index]
			if _, exists := bindings[entry[0]]; exists {
				return nil, fmt.Errorf("%s: duplicate annotation %s", name, entry[0])
			}
			bindings[entry[0]] = entry[1]
		}
		snapshot[name] = bindings
	}
	return snapshot, nil
}

func writeInferenceArchive(path string, captured inferenceCapture) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	compressed := gzip.NewWriter(file)
	encodeErr := json.NewEncoder(compressed).Encode(compactInference(captured))
	gzipErr := compressed.Close()
	return errors.Join(encodeErr, gzipErr, file.Close())
}

func readInferenceArchive(path string) (inferenceSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer compressed.Close()
	// Reading through EOF also verifies the gzip checksum before comparison.
	data, err := io.ReadAll(compressed)
	if err != nil {
		return nil, err
	}
	var archive inferenceArchive
	if err := json.Unmarshal(data, &archive); err != nil {
		return nil, err
	}
	return archive.expand()
}

func TestInferenceBaselineArchive(t *testing.T) {
	captured := inferenceCapture{
		Types: inferenceSnapshot{
			"first":  {"file0/prelude": "func(A) A", "file1/let": "func(Let20) Let20"},
			"second": {"file0/prelude": "func(A) A", "file1/let": "Int"},
			"third":  {"file0/prelude": "func(B) B"},
		},
		Libraries: map[string][]string{
			"first": {"file0/prelude"}, "second": {"file0/prelude"}, "third": {"file0/prelude"},
		},
	}
	archive := compactInference(captured)
	if len(archive.Shared) != 2 || len(archive.Fixtures["first"].Bindings) != 1 {
		t.Fatalf("library annotations were not deduplicated: %#v", archive)
	}
	path := filepath.Join(t.TempDir(), "baseline.json.gz")
	if err := writeInferenceArchive(path, captured); err != nil {
		t.Fatal(err)
	}
	restored, err := readInferenceArchive(path)
	if err != nil || !reflect.DeepEqual(restored, captured.Types) {
		t.Fatalf("lossless round trip = %v, %v; want %v", restored, err, captured.Types)
	}
	archive.Fixtures["bad"] = inferenceArchiveFixture{Shared: []int{len(archive.Shared)}}
	if _, err := archive.expand(); err == nil {
		t.Fatal("accepted an invalid shared reference")
	}
	archive.Fixtures["bad"] = inferenceArchiveFixture{Shared: []int{0}, Bindings: map[string]string{"file0/prelude": "Int"}}
	if _, err := archive.expand(); err == nil {
		t.Fatal("accepted a duplicated binding")
	}
}
