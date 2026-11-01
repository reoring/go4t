package storage

import (
	"strings"
	"testing"

	"attachrepairctl/decide"

	"gofx/rt"
)

func TestAttachmentRepairDecisionProtocol(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	fixture.addExactApproval(t)
	approval := projectApproval(fixture.operation, fixture.approval)
	approval.Phase = "Accepted"
	staleIdentity := attachmentRepairObjectIdentityDigest("volume-attachment-v1", fixture.stale)
	base := decide.Observed{
		Mode:                      decide.ModeLive{F0: decide.ApprovalReadRead{F0: approval}},
		FeatureGateEnabled:        true,
		OperationGeneration:       fixture.operation.GetGeneration(),
		ApprovedObservationDigest: approval.Digest,
		NowNanos:                  attachmentRepairTestNow.UnixNano(),
		ApprovalMaxAgeNanos:        int64(attachmentRepairApprovalMaxAge),
	}
	dryRun := base
	dryRun.Mode = decide.ModeDryRun{}
	noRef := base
	noRef.Mode = decide.ModeLive{F0: decide.ApprovalReadNoRef{}}
	gateDisabled := base
	gateDisabled.FeatureGateEnabled = false
	authorized := base
	authorized.Authorized = true
	committed := authorized
	committed.Committed = true
	blockedInitial := base
	blockedInitial.Initial = rt.Option[decide.Observation]{Present: true, Value: decide.ObservationObsBlocked{
		F0: decide.ObsReasonPodTargetNotUnique, F1: "initial observation blocked",
	}}
	blockedAuthorized := authorized
	blockedAuthorized.AuthorizedObs = rt.Option[decide.AuthorizedObservation]{Present: true, Value: decide.AuthorizedObservationAuthBlocked{
		F0: decide.ObsReasonAttachmentSetChanged, F1: "authorized observation blocked",
	}}
	tests := []struct {
		name       string
		observed   decide.Observed
		deletion   decide.DeleteResult
		wantTrace  string
		wantReason string
	}{
		{"approval rejected before observation", noRef, decide.DeleteResultDeleted, "Publish", "ApprovalReferenceMissing"},
		{"dry run", dryRun, decide.DeleteResultDeleted, "ObserveInitial,Publish", ""},
		{"authorize", base, decide.DeleteResultDeleted, "ObserveInitial,Publish", ""},
		{"matched approval with disabled gate", gateDisabled, decide.DeleteResultDeleted, "ObserveInitial,Publish", "FeatureGateDisabled"},
		{"initial observation blocked", blockedInitial, decide.DeleteResultDeleted, "Publish", "PodTargetNotUnique"},
		{"authorized observation blocked", blockedAuthorized, decide.DeleteResultDeleted, "Publish", "AttachmentSetChanged"},
		{"commit", authorized, decide.DeleteResultDeleted, "ObserveAuthorized,Publish", ""},
		{"delete", committed, decide.DeleteResultDeleted, "ObserveAuthorized,DeleteStale,Publish", ""},
		{"compare and swap failure", committed, decide.DeleteResultCasFailed, "ObserveAuthorized,DeleteStale,Publish", "DeleteCompareAndSwapFailed"},
		{"delete failure", committed, decide.DeleteResultDeleteFailed, "ObserveAuthorized,DeleteStale,Publish", "DeleteRequestFailed"},
	}
	maxIterations := 0
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := tt.observed
			var trace []string
			for {
				if len(trace) == 3 {
					t.Fatal("decision exceeded three iterations")
				}
				switch a := decide.Decide(o).(type) {
				case decide.ActionObserveInitial:
					if o.Initial.Present {
						t.Fatal("decide asked twice")
					}
					trace = append(trace, "ObserveInitial")
					o.Initial = rt.Option[decide.Observation]{Present: true, Value: decide.ObservationObsOk{F0: approval.Digest}}
				case decide.ActionObserveAuthorized:
					if o.AuthorizedObs.Present {
						t.Fatal("decide asked twice")
					}
					trace = append(trace, "ObserveAuthorized")
					o.AuthorizedObs = rt.Option[decide.AuthorizedObservation]{Present: true, Value: decide.AuthorizedObservationAuthOk{F0: decide.AuthOkObs{
						Target: decide.TargetPresent, Digest: approval.Digest, StaleIdentity: staleIdentity,
					}}}
				case decide.ActionDeleteStale:
					if o.DeleteResult.Present {
						t.Fatal("decide asked twice")
					}
					if a.F0 != staleIdentity {
						t.Fatalf("delete identity = %q, want %q", a.F0, staleIdentity)
					}
					trace = append(trace, "DeleteStale")
					o.DeleteResult = rt.Option[decide.DeleteResult]{Present: true, Value: tt.deletion}
				case decide.ActionPublish:
					trace = append(trace, "Publish")
					if got := strings.Join(trace, ","); got != tt.wantTrace {
						t.Fatalf("trace = %s, want %s", got, tt.wantTrace)
					}
					if tt.wantReason != "" {
						blocked, ok := a.F0.(decide.StatusBlocked)
						if !ok || reasonString(blocked.Reason) != tt.wantReason {
							t.Fatalf("status = %#v, want blocked reason %s", a.F0, tt.wantReason)
						}
						if tt.wantReason == "FeatureGateDisabled" {
							obs := &attachmentRepairObservation{safety: attachmentRepairSafetyStatus()}
							r := fixture.operationReconciler(nil, false)
							status, _ := r.publish(fixture.operation, blocked, obs, fixture.approval)
							if status["safety"].(map[string]any)["approvalMatchesObservation"] != true {
								t.Fatal("feature-gate block lost matching-approval evidence")
							}
						}
					}
					if len(trace) > maxIterations {
						maxIterations = len(trace)
					}
					t.Logf("iterations=%d trace=%s", len(trace), strings.Join(trace, ","))
					return
				default:
					t.Fatalf("unknown action %T", a)
				}
			}
		})
	}
	if maxIterations != 3 {
		t.Fatalf("max iterations = %d, want 3", maxIterations)
	}
	t.Logf("max decision iterations observed: %d", maxIterations)
}

func TestAttachmentRepairDeleteIdentityGuard(t *testing.T) {
	fixture := newAttachmentRepairFixture(false)
	obs := &attachmentRepairObservation{staleAttachment: fixture.stale}
	identity := attachmentRepairObjectIdentityDigest("volume-attachment-v1", fixture.stale)
	tests := []struct {
		name      string
		observed  *attachmentRepairObservation
		identity  string
		wantPanic bool
	}{
		{"matching identity", obs, identity, false},
		{"mismatched identity", obs, "different-identity", true},
		{"missing authorized observation", nil, identity, true},
		{"missing stale attachment", &attachmentRepairObservation{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if got := recover(); got != "decide named a stale attachment that was not observed" {
						t.Fatalf("panic = %v, want stale attachment identity guard", got)
					}
				}()
			}
			got := observedStaleAttachment(tt.observed, tt.identity)
			if tt.wantPanic {
				t.Fatal("delete identity guard did not panic")
			}
			if got != fixture.stale {
				t.Fatal("delete identity guard did not return the observed attachment")
			}
		})
	}
}
