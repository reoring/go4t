package host

import (
 "context"

 "go4t/rt"
)

// Nonzero size ensures distinct allocated keys have distinct addresses.
// No payload representation or type-name interpretation occurs at this boundary.
type payloadKey struct { marker byte }
type payloadBox struct { key *payloadKey; value any }

// An immutable built-in witness; allocation and payload policy stay native.
var runtimeObjectKey payloadKey
func RuntimeObjectPayloadKey() any { return &runtimeObjectKey }

func NewPayloadKey() any {
 return rt.IO(func(context.Context) (any, error) { return &payloadKey{}, nil })
}

func PackPayload(key, value any) any {
 return &payloadBox{key: key.(*payloadKey), value: value}
}

func PayloadValue(key, value any) any {
 witness, validKey := key.(*payloadKey)
 box, validBox := value.(*payloadBox)
 if !validKey || !validBox || witness != box.key {
  return rt.Shapes.None()
 }
 return rt.Shapes.Some(box.value)
}
