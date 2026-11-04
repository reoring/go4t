package drain

import (
	"reflect"
	"testing"

	"github.com/reoring/go4t/rt"

	"nodedrainctl/decide"
)

type fakeCluster struct {
	nodes    map[string]decide.NodeState
	cordoned map[string]bool
	pods     map[string][]decide.Pod
	blocked  map[string]int // pod -> remaining refusals
	now      int64
	log      []string
}

func newCluster() *fakeCluster {
	return &fakeCluster{
		nodes:    map[string]decide.NodeState{},
		cordoned: map[string]bool{},
		pods:     map[string][]decide.Pod{},
		blocked:  map[string]int{},
	}
}

func (c *fakeCluster) NodeState(n string) (decide.NodeState, bool) { s, ok := c.nodes[n]; return s, ok }
func (c *fakeCluster) CordonedBy(n string) bool                   { return c.cordoned[n] }
func (c *fakeCluster) Pods(n string) []decide.Pod                 { return c.pods[n] }
func (c *fakeCluster) Now() int64                                 { return c.now }

func (c *fakeCluster) Cordon(n string) error {
	c.log = append(c.log, "cordon "+n)
	c.nodes[n] = decide.NodeStateCordoned
	c.cordoned[n] = true
	return nil
}

func (c *fakeCluster) Uncordon(n string) error {
	c.log = append(c.log, "uncordon "+n)
	c.nodes[n] = decide.NodeStateSchedulable
	delete(c.cordoned, n)
	return nil
}

func (c *fakeCluster) Evict(n, pod string) error {
	c.log = append(c.log, "evict "+pod)
	if c.blocked[pod] > 0 {
		c.blocked[pod]--
		return &EvictionBlocked{Message: "pdb " + pod}
	}
	kept := c.pods[n][:0:0]
	for _, p := range c.pods[n] {
		if p.Name != pod {
			kept = append(kept, p)
		}
	}
	c.pods[n] = kept
	return nil
}

// run reconciles until the drain leaves Pending or the step budget is spent.
func run(t *testing.T, c *fakeCluster, d *Drain, steps int) []decide.Action {
	t.Helper()
	var actions []decide.Action
	for i := range steps {
		a, err := Reconcile(c, d)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		actions = append(actions, a)
		if d.Phase != PhasePending {
			return actions
		}
		c.now += 10
	}
	t.Fatalf("still %s after %d steps: %v", d.Phase, steps, actions)
	return nil
}

func TestDrainSkipsDaemonSetAndMirrorPods(t *testing.T) {
	c := newCluster()
	c.nodes["n1"] = decide.NodeStateSchedulable
	c.pods["n1"] = []decide.Pod{
		{Name: "kube-proxy", Kind: decide.PodKindDaemonSet},
		{Name: "web-a", Kind: decide.PodKindManaged},
		{Name: "etcd", Kind: decide.PodKindMirror},
		{Name: "web-b", Kind: decide.PodKindManaged},
	}
	d := &Drain{Node: "n1", Requested: true, Phase: PhasePending}

	actions := run(t, c, d, 10)

	want := []decide.Action{
		decide.ActionCordon{},
		decide.ActionEvict{F0: "web-a"},
		decide.ActionEvict{F0: "web-b"},
		decide.ActionMarkDrained{},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions %v, want %v", actions, want)
	}
	if len(c.pods["n1"]) != 2 {
		t.Fatalf("daemonset/mirror pods must remain: %v", c.pods["n1"])
	}
}

func TestPDBBlockWaitsThenRetries(t *testing.T) {
	c := newCluster()
	c.nodes["n1"] = decide.NodeStateCordoned
	c.cordoned["n1"] = true
	c.pods["n1"] = []decide.Pod{{Name: "db-0", Kind: decide.PodKindManaged}}
	c.blocked["db-0"] = 1
	d := &Drain{Node: "n1", Requested: true, Phase: PhasePending}

	actions := run(t, c, d, 10)

	want := []decide.Action{
		decide.ActionEvict{F0: "db-0"}, // refused by PDB
		decide.ActionWait{F0: "pdb db-0"},
		decide.ActionEvict{F0: "db-0"},
		decide.ActionMarkDrained{},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions %v, want %v", actions, want)
	}
}

func TestDeadlineFailsRegardlessOfPods(t *testing.T) {
	c := newCluster()
	c.nodes["n1"] = decide.NodeStateCordoned
	c.cordoned["n1"] = true
	c.pods["n1"] = []decide.Pod{{Name: "db-0", Kind: decide.PodKindManaged}}
	c.blocked["db-0"] = 100
	c.now = 0
	d := &Drain{Node: "n1", Requested: true, Phase: PhasePending, DeadlineNanos: rt.Some[int64](25)}

	actions := run(t, c, d, 10)

	last := actions[len(actions)-1]
	if !reflect.DeepEqual(last, decide.ActionMarkFailed{F0: decide.FailReasonTimedOut}) {
		t.Fatalf("last action %v", last)
	}
	if d.Phase != PhaseFailed || d.Message != "drain deadline exceeded" {
		t.Fatalf("phase %s message %q", d.Phase, d.Message)
	}
	if c.now < 25 {
		t.Fatalf("failed before deadline at now=%d", c.now)
	}
}

func TestWithdrawnRequestUncordonsOnlyOurNode(t *testing.T) {
	ours := newCluster()
	ours.nodes["n1"] = decide.NodeStateCordoned
	ours.cordoned["n1"] = true
	d := &Drain{Node: "n1", Requested: false, Phase: PhasePending}
	if a, _ := Reconcile(ours, d); !reflect.DeepEqual(a, decide.ActionUncordon{}) || d.Phase != PhaseReleased {
		t.Fatalf("ours: action %v phase %s", a, d.Phase)
	}

	theirs := newCluster()
	theirs.nodes["n2"] = decide.NodeStateCordoned // cordoned by an operator, not us
	d2 := &Drain{Node: "n2", Requested: false, Phase: PhasePending}
	if a, _ := Reconcile(theirs, d2); !reflect.DeepEqual(a, decide.ActionNoop{}) || len(theirs.log) != 0 {
		t.Fatalf("theirs: action %v log %v", a, theirs.log)
	}
}

func TestMissingNodeFails(t *testing.T) {
	c := newCluster()
	d := &Drain{Node: "ghost", Requested: true, Phase: PhasePending}
	if _, err := Reconcile(c, d); err != nil {
		t.Fatal(err)
	}
	if d.Phase != PhaseFailed {
		t.Fatalf("phase %s", d.Phase)
	}
}
