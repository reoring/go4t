// Package drain is the Go shell around the go4t decision core in decide/.
// It observes, decides, and applies exactly one action per Reconcile.
package drain

import (
	"errors"
	"fmt"

	"github.com/reoring/go4t/rt"

	"nodedrainctl/decide"
)

// Cluster is the effectful boundary the controller drives.
type Cluster interface {
	NodeState(node string) (decide.NodeState, bool) // ok=false: node missing
	CordonedBy(node string) bool
	Pods(node string) []decide.Pod
	Cordon(node string) error
	Uncordon(node string) error
	Evict(node, pod string) error // returns *EvictionBlocked when a PDB refuses
	Now() int64
}

// EvictionBlocked is returned by Cluster.Evict when a PodDisruptionBudget
// refuses the eviction; the controller waits instead of failing.
type EvictionBlocked struct{ Message string }

func (e *EvictionBlocked) Error() string { return "eviction blocked: " + e.Message }

// Drain is the controller's view of one NodeDrain object.
type Drain struct {
	Node          string
	Requested     bool
	DeadlineNanos rt.Option[int64]

	// Status written by the controller.
	Phase      Phase
	Message    string
	lastBlock  rt.Option[string]
	lastAction decide.Action
}

type Phase string

const (
	PhasePending Phase = "Pending"
	PhaseDrained Phase = "Drained"
	PhaseFailed  Phase = "Failed"
	PhaseReleased Phase = "Released"
)

// Reconcile observes the cluster, decides, applies one action and records it.
// It returns the action so callers can requeue on Evict/Wait.
func Reconcile(c Cluster, d *Drain) (decide.Action, error) {
	o := observe(c, d)
	action := decide.Decide(o)
	d.lastAction = action
	switch a := action.(type) {
	case decide.ActionNoop:
		if !d.Requested && d.Phase == PhasePending {
			d.Phase = PhaseReleased
		}
	case decide.ActionCordon:
		return action, c.Cordon(d.Node)
	case decide.ActionUncordon:
		if err := c.Uncordon(d.Node); err != nil {
			return action, err
		}
		d.Phase = PhaseReleased
	case decide.ActionEvict:
		err := c.Evict(d.Node, a.F0)
		var blocked *EvictionBlocked
		if errors.As(err, &blocked) {
			d.lastBlock = rt.Some(blocked.Message)
			return action, nil
		}
		d.lastBlock = rt.None[string]()
		return action, err
	case decide.ActionWait:
		d.Message = a.F0
		d.lastBlock = rt.None[string]() // retry the eviction next time
	case decide.ActionMarkDrained:
		d.Phase = PhaseDrained
	case decide.ActionMarkFailed:
		d.Phase = PhaseFailed
		d.Message = failMessage(a.F0)
	default:
		return action, fmt.Errorf("unknown action %T", action)
	}
	return action, nil
}

func observe(c Cluster, d *Drain) decide.Observed {
	node, present := c.NodeState(d.Node)
	if !present {
		node = decide.NodeStateMissing
	}
	var deadline decide.Deadline = decide.DeadlineNoDeadline{}
	if d.DeadlineNanos.Present {
		deadline = decide.DeadlineDeadlineAt{F0: d.DeadlineNanos.Value}
	}
	return decide.Observed{
		Requested:       d.Requested,
		Node:            node,
		CordonedByUs:    c.CordonedBy(d.Node),
		Pods:            c.Pods(d.Node),
		EvictionBlocked: d.lastBlock,
		Deadline:        deadline,
		NowNanos:        c.Now(),
	}
}

func failMessage(r decide.FailReason) string {
	switch r {
	case decide.FailReasonNodeMissing:
		return "node not found"
	case decide.FailReasonTimedOut:
		return "drain deadline exceeded"
	}
	return "unknown failure"
}
