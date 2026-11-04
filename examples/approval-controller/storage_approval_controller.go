package storage

import (
	"context"
	"fmt"
	"time"

	decide "approvalctl/_decide_gen"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const StorageApprovalKind = "StorageApproval"

var storageApprovalResourceGVK = schema.GroupVersionKind{
	Group:   StorageCapabilityGroup,
	Version: StorageCapabilityVersion,
	Kind:    StorageApprovalKind,
}

type ApprovalReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Now      func() time.Time
}

func (r *ApprovalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "storageApproval", req.NamespacedName)
	if r.Log.GetSink() != nil {
		logger = r.Log.WithValues("storageApproval", req.NamespacedName)
	}

	approval := newStorageApprovalObject()
	if err := r.Client.Get(ctx, req.NamespacedName, approval); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get StorageApproval: %w", err)
	}
	if !approval.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	verdict := r.observeVerdict(ctx, approval)
	outcome, err := decide.Apply(r.Client, approval, verdict)(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if r.Recorder != nil {
		for _, event := range outcome.Events {
			r.Recorder.Event(approval, event.EventType, event.Reason, event.Message)
		}
	}
	logger.V(1).Info("published storage approval status")
	return ctrl.Result{RequeueAfter: time.Duration(outcome.RequeueNanos)}, nil
}

func (r *ApprovalReconciler) observeVerdict(ctx context.Context, approval *unstructured.Unstructured) decide.Verdict {
	inputs, err := decide.ObserveInputs(r.Client, approval)(ctx)
	var verdict decide.Verdict
	if err != nil {
		verdict = decide.VerdictPending{
			F0: decide.ReasonPlanReadFailed,
			F1: "Storage approval inputs could not be observed: " + err.Error(),
			F2: int64(time.Minute),
		}
	} else {
		if r.Now != nil {
			inputs.NowNanos = r.Now().UTC().UnixNano()
		}
		verdict = decide.Decide(inputs)
	}
	return verdict
}

func (r *ApprovalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(newStorageApprovalObject()).
		Complete(r)
}
