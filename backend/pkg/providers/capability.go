package providers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"opsagi/pkg/capability"
	"opsagi/pkg/database"
	"opsagi/pkg/tools"

	"github.com/sirupsen/logrus"
)

// capabilityRecordTimeout bounds the audit write. The write must not hold up the
// tool call it describes, and a stuck database must not turn a refusal into a
// timeout of its own.
const capabilityRecordTimeout = 5 * time.Second

// capabilityGuard builds the lease check for one subtask. It returns nil when
// the flow was created with the switch off, which is the whole point of the
// switch: an instance that never opted in runs the code path it always ran.
//
// A failure to read the flow or issue the lease disables the guard instead of
// failing the run. The guard is an addition; a database that cannot answer must
// not turn a working diagnosis into an error, and the failure is logged loudly
// because it means the flow runs unguarded.
func (fp *flowProvider) capabilityGuard(ctx context.Context, subtaskID int64, role string) *tools.CapabilityGuard {
	flow, err := fp.db.GetFlow(ctx, fp.flowID)
	if err != nil {
		logrus.WithContext(ctx).WithError(err).WithField("flow_id", fp.flowID).
			Warn("capability: cannot read the flow, this subtask runs without a lease")
		return nil
	}

	mode := capability.ParseMode(flow.CapabilityMode)
	if !mode.Enabled() {
		return nil
	}

	subtask, err := fp.db.GetSubtask(ctx, subtaskID)
	if err != nil {
		logrus.WithContext(ctx).WithError(err).WithField("subtask_id", subtaskID).
			Warn("capability: cannot read the subtask, this subtask runs without a lease")
		return nil
	}

	planMode := fp.subtaskMode(ctx, subtask)

	lease, err := fp.ensureLease(ctx, planMode, subtaskID, role)
	if err != nil {
		logrus.WithContext(ctx).WithError(err).WithField("subtask_id", subtaskID).
			Warn("capability: cannot issue the lease, this subtask runs without a lease")
		return nil
	}

	return tools.NewCapabilityGuard(tools.CapabilityGuardOptions{
		Lease:   lease,
		Mode:    mode,
		Grants:  fp.capabilityGrants(lease),
		Record:  fp.capabilityRecorder(lease, role),
		Request: fp.capabilityRequester(lease, role),
	})
}

// capabilityGrants reloads the flow's approvals on every call. Reading them per
// call is what makes an approval land on the running subtask's next attempt
// instead of its next life.
func (fp *flowProvider) capabilityGrants(lease *capability.Lease) func(ctx context.Context) []capability.Grant {
	return func(ctx context.Context) []capability.Grant {
		rows, err := fp.db.ListActiveCapabilityGrants(ctx, lease.FlowID)
		if err != nil {
			logrus.WithContext(ctx).WithError(err).WithField("flow_id", lease.FlowID).
				Warn("capability: cannot read the approvals, this call is judged by the lease alone")

			return nil
		}

		grants := make([]capability.Grant, 0, len(rows))
		for _, row := range rows {
			grants = append(grants, capability.Grant{
				ID:           row.ID,
				Tool:         row.Tool,
				Target:       row.Target,
				Command:      row.Command,
				Shape:        row.Shape,
				TraceProgram: row.TraceProgram,
				Reason:       row.Reason,
				IssuedBy:     row.IssuedBy,
				ExpiresAt:    row.ExpiresAt,
			})
		}

		return grants
	}
}

// capabilityRequester files the approval a refusal is asking for. One pending
// request per distinct call is enough: a model that retries the same refusal
// must not bury the operator in copies of it.
func (fp *flowProvider) capabilityRequester(
	lease *capability.Lease,
	role string,
) func(ctx context.Context, call capability.Call, reason string) (int64, error) {
	return func(ctx context.Context, call capability.Call, reason string) (int64, error) {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), capabilityRecordTimeout)
		defer cancel()

		lookup := database.FindPendingCapabilityRequestParams{
			FlowID:       lease.FlowID,
			Tool:         call.Tool,
			Target:       call.Target,
			Command:      call.Command,
			Shape:        call.Shape,
			TraceProgram: call.TraceProgram,
		}

		pending, err := fp.db.FindPendingCapabilityRequest(writeCtx, lookup)
		switch {
		case err == nil:
			return pending.ID, nil
		case !errors.Is(err, sql.ErrNoRows):
			return 0, err
		}

		created, err := fp.db.CreateCapabilityRequest(writeCtx, database.CreateCapabilityRequestParams{
			FlowID:       lease.FlowID,
			SubtaskID:    sql.NullInt64{Int64: lease.SubtaskID, Valid: true},
			Role:         role,
			Tool:         call.Tool,
			Target:       call.Target,
			Command:      call.Command,
			Shape:        call.Shape,
			TraceProgram: call.TraceProgram,
			Reason:       reason,
		})
		if err != nil {
			return 0, err
		}

		logrus.WithContext(ctx).WithFields(logrus.Fields{
			"flow_id":    lease.FlowID,
			"subtask_id": lease.SubtaskID,
			"tool":       call.Tool,
			"target":     call.Target,
		}).Info("capability: refusal filed as an approval request")

		return created.ID, nil
	}
}

// ensureLease returns the subtask's lease, issuing it once if this is the first
// execution. The row is reused on restart, so a resumed subtask keeps the scope
// it was given instead of being widened by a second issue.
func (fp *flowProvider) ensureLease(
	ctx context.Context,
	planMode string,
	subtaskID int64,
	role string,
) (*capability.Lease, error) {
	if row, err := fp.db.GetCapabilityLease(ctx, subtaskID); err == nil {
		return leaseFromRow(row), nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	issued := capability.DefaultPolicy().Issue(planMode, fp.flowID, subtaskID, role, time.Now())

	row, err := fp.db.GetOrCreateCapabilityLease(ctx, database.GetOrCreateCapabilityLeaseParams{
		FlowID:        issued.FlowID,
		SubtaskID:     issued.SubtaskID,
		Role:          issued.Role,
		Mode:          planMode,
		Targets:       issued.Targets,
		Tools:         issued.Tools,
		Commands:      issued.Commands,
		Shapes:        issued.Shapes,
		TracePrograms: issued.TracePrograms,
		IssuedBy:      issued.IssuedBy,
		ExpiresAt:     issued.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	return leaseFromRow(row), nil
}

func leaseFromRow(row database.CapabilityLease) *capability.Lease {
	return &capability.Lease{
		ID:            row.ID,
		FlowID:        row.FlowID,
		SubtaskID:     row.SubtaskID,
		Role:          row.Role,
		Mode:          capability.ParseMode(row.Mode),
		Targets:       row.Targets,
		Tools:         row.Tools,
		Commands:      row.Commands,
		Shapes:        row.Shapes,
		TracePrograms: row.TracePrograms,
		IssuedBy:      row.IssuedBy,
		ExpiresAt:     row.ExpiresAt,
	}
}

// capabilityRecorder writes the decision down, including the verdict the guard
// already resolved: a shadow-mode refusal arrives as would_deny, which is what
// makes the mode useful — the operator sees exactly what enforcement would have
// stopped, without any of it stopping.
func (fp *flowProvider) capabilityRecorder(
	lease *capability.Lease,
	role string,
) func(ctx context.Context, call capability.Call, decision capability.Decision) {
	return func(ctx context.Context, call capability.Call, decision capability.Decision) {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), capabilityRecordTimeout)
		defer cancel()

		err := fp.db.RecordCapabilityDecision(writeCtx, database.RecordCapabilityDecisionParams{
			FlowID:    fp.flowID,
			SubtaskID: sql.NullInt64{Int64: lease.SubtaskID, Valid: true},
			LeaseID:   sql.NullInt64{Int64: lease.ID, Valid: true},
			Role:      role,
			Tool:      call.Tool,
			Target:    call.Target,
			Command:   call.Command,
			Verdict:   string(decision.Verdict),
			Reason:    decision.Reason,
		})
		if err != nil {
			logrus.WithContext(ctx).WithError(err).WithFields(logrus.Fields{
				"flow_id":    fp.flowID,
				"subtask_id": lease.SubtaskID,
				"tool":       call.Tool,
			}).Warn("capability: cannot record the decision")
		}
	}
}
