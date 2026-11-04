package jsonview

import (
	"encoding/json"
	"errors"
	"time"
)

type Stamp struct{ t time.Time }

func (s Stamp) MarshalJSON() ([]byte, error) { return json.Marshal(s.t.UTC().Format(time.RFC3339)) }
func (s *Stamp) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return err
	}
	s.t = parsed
	return nil
}
func (s Stamp) Location() string { return s.t.Location().String() }
func Same(a, b *Stamp) bool      { return a == b }

type Pair struct{ A, B int }

func (p Pair) MarshalJSON() ([]byte, error) { return json.Marshal([2]int{p.A, p.B}) }
func (p *Pair) UnmarshalJSON(data []byte) error {
	var values [2]int
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	p.A, p.B = values[0], values[1]
	return nil
}

type Holder struct {
	When Stamp
	N    int
}

type Nested struct {
	Holders []Holder
	Maybe   *Holder
	Stamps  map[string]Stamp
}

func EchoHolder(v Holder) Holder { return v }

type Number int

func (n Number) MarshalJSON() ([]byte, error) { return json.Marshal(int(n)) }
func (n *Number) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, (*int)(n))
}

var InvalidJSON = errors.New("invalid fixture JSON")

type Invalid struct{ private int }

func (Invalid) MarshalJSON() ([]byte, error) { return nil, InvalidJSON }
func (*Invalid) UnmarshalJSON([]byte) error { return InvalidJSON }

type Malformed struct{ private int }

func (Malformed) MarshalJSON() ([]byte, error) { return []byte("{"), nil }
func (*Malformed) UnmarshalJSON([]byte) error { return nil }

type PointerOnly struct{ private int }

func (*PointerOnly) MarshalJSON() ([]byte, error) { return []byte("null"), nil }
func (*PointerOnly) UnmarshalJSON([]byte) error   { return nil }

type MarshalOnly struct{ private int }

func (MarshalOnly) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

type UnmarshalOnly struct{ private int }

func (*UnmarshalOnly) UnmarshalJSON([]byte) error { return nil }

type Generic[T any] struct{ private T }

func (Generic[T]) MarshalJSON() ([]byte, error) { return []byte("null"), nil }
func (*Generic[T]) UnmarshalJSON([]byte) error  { return nil }
