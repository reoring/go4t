package main

//go:generate go run ../../cmd/go4t build --emit-go core_gen --package core core

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"nodedrainshell/core_gen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "--release") {
		return fmt.Errorf("usage: node-drain-shell node kubeconfig [--release]")
	}
	config, err := clientcmd.BuildConfigFromFlags("", args[1])
	if err != nil {
		return err
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	drain := core.Drain{
		Node:      args[0],
		Requested: len(args) == 2,
		Deadline:  core.DeadlineNoDeadline{},
	}
	ctx := context.Background()
	for attempt := range 60 {
		outcome, err := core.Reconcile(cs, drain)(ctx)
		if err != nil {
			return err
		}
		drain = outcome.Drain
		fmt.Println(outcome.Action)
		if drain.LastBlock.Present {
			fmt.Fprintln(os.Stderr, "blocked:", drain.LastBlock.Value)
		}
		switch action := outcome.Action.(type) {
		case core.ActionMarkDrained:
			return nil
		case core.ActionNoop:
			if !drain.Requested {
				return nil
			}
		case core.ActionMarkFailed:
			return fmt.Errorf("drain failed: %s", action.F0)
		}
		if attempt < 59 {
			time.Sleep(2 * time.Second)
		}
	}
	return fmt.Errorf("drain did not finish after 60 attempts")
}
