package lockfixture

import "sync"

type ReadOnly struct { Label string }

func (r ReadOnly) ReadLabel() string { return r.Label }

type Guarded struct {
	mu sync.Mutex
	N int
	ReadOnly
}

func (g *Guarded) Inc() { g.mu.Lock(); g.N++; g.mu.Unlock() }

var Global Guarded

type Holder struct { G Guarded }

type Plain struct { private int; N int }

var PlainGlobal Plain

type PlainHolder struct { P Plain }

// Private state keeps this accessor-identity fixture opaque even though a
// record may now contain a Plain handle by value.
type PrivateHolder struct { P Plain; private bool }

func Take(g Guarded) {}
func Make() Guarded { return Guarded{} }
func TakePointer(g *Guarded) {}
func TakeSlice(g []Guarded) {}
func MakeMap() map[string]Guarded { return nil }
func TakeChannel(g chan Guarded) {}
func TakeCallback(f func(Guarded)) {}
func MakeCallback() func() Guarded { return Make }

type ArrayGuarded struct { Locks [2]sync.Mutex }
func TakeArray(g ArrayGuarded) {}

type EmbeddedGuarded struct { sync.Mutex }
func TakeEmbedded(g EmbeddedGuarded) {}

type LockerHolder struct { sync.Locker }
func TakeLocker(g LockerHolder) {}

type ValueLocker struct { N int }
func (ValueLocker) Lock() {}
func (ValueLocker) Unlock() {}
func TakeValueLocker(g ValueLocker) {}

type GuardedChan chan Guarded

var GlobalSlice []Guarded
var LockedChannel GuardedChan

type StreamHolder struct { Stream chan Guarded }
