//go:build ignore

package main

//go:generate gofx build --emit-go core --package core core.gfx

import (
	"fmt"
	"reflect"

	"gofx/rt"
	"goshell/core"
)

func main() {
	observed := core.Observed{
		ClassMatches: true,
		PendingDeps:  []string{"cache", "secrets"},
		ValuesValid:  true,
	}
	switch step := core.Decide(observed).(type) {
	case core.StepDependenciesPending:
		fmt.Println("pending:", step.F0)
	default:
		panic("expected pending dependencies")
	}
	observed.Deleting = true
	switch step := core.Decide(observed).(type) {
	case core.StepCleanup:
		fmt.Printf("cleanup: %v\n", step)
	default:
		panic("expected cleanup")
	}
	copied := core.EchoObserved(observed)
	if !reflect.DeepEqual(copied, observed) {
		panic("record round trip")
	}
	copied.PendingDeps[0] = "changed"
	if observed.PendingDeps[0] != "cache" {
		panic("list aliasing")
	}
	phase := core.EchoPhase(core.PhaseReady)
	if phase != core.PhaseReady || core.EchoPhase(0) != core.PhasePending {
		panic("enum round trip")
	}
	option := core.EchoOption(rt.Some(observed.PendingDeps))
	if !option.Present || !reflect.DeepEqual(option.Value, observed.PendingDeps) || core.EchoOption(rt.None[[]string]()).Present {
		panic("option round trip")
	}
	option.Value[0] = "changed"
	if observed.PendingDeps[0] != "cache" {
		panic("option list aliasing")
	}
	fmt.Println("round trips:", phase, "record list option")
}
