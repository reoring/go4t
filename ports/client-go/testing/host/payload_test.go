package host

import (
 "context"
 "testing"

 "gofx/rt"
)

func TestPayloadWitnessIdentityAndShallowValue(t *testing.T) {
 oldSome, oldNone := rt.Shapes.Some, rt.Shapes.None
 type present struct { value any }
 type absent struct{}
 rt.Shapes.Some = func(value any) any { return present{value} }
 rt.Shapes.None = func() any { return absent{} }
 defer func() { rt.Shapes.Some, rt.Shapes.None = oldSome, oldNone }()

 newKey := func() any {
  key, err := NewPayloadKey().(rt.IO)(context.Background())
  if err != nil { t.Fatal(err) }
  return key
 }
 key, other := newKey(), newKey()
 value := 1
 builtin := RuntimeObjectPayloadKey()
 if builtin != RuntimeObjectPayloadKey() { t.Fatal("built-in payload key is not stable") }
 if builtin == key || builtin == other { t.Fatal("fresh key aliases the built-in key") }
 box := PackPayload(key, &value)
 got := PayloadValue(key, box).(present).value.(*int)
 *got = 2
 if value != 2 { t.Fatal("payload reference was copied") }
 if _, ok := PayloadValue(other, box).(absent); !ok { t.Fatal("distinct same-type key accepted") }
 if _, ok := PayloadValue(nil, box).(absent); !ok { t.Fatal("invalid key accepted") }
 if _, ok := PayloadValue(key, nil).(absent); !ok { t.Fatal("invalid payload accepted") }
 callable := func() { value++ }
 PayloadValue(key, PackPayload(key, callable)).(present).value.(func())()
 if value != 3 { t.Fatal("captured function identity was lost") }
}
