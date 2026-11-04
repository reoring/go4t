package nodedraintyped

// Keeps the modules the go: imports and generated shims need in go.sum.
import (
	_ "github.com/reoring/go4t/rt"
	_ "k8s.io/api/policy/v1"
	_ "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/tools/clientcmd"
)
