package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/span"
)

// A trace of session work is meant to be read from top to bottom: the request,
// the operation it started, each check with its outcome, the commands that
// ran. Small operations (start an exam, store an answer, open a hint) get no
// span of their own; the API names them on the request span.

// tracer is the tracer of the manager, or the one the context brought along.
func (m *Manager) tracer(ctx context.Context) span.Tracer {
	if m.hooks.Tracer != nil {
		return m.hooks.Tracer
	}
	return span.From(ctx)
}

// span starts a span for work on a session. The returned context carries the
// tracer, so the checker and the sandbox driver trace below it with
// span.Start. The session is an attribute of spans and log lines only;
// metrics never carry it.
func (m *Manager) span(ctx context.Context, name string, s *Session, attrs ...span.Attr) (context.Context, span.Span) {
	all := append([]span.Attr{
		span.String("kubedojo.session.id", s.ID),
		span.String("kubedojo.track", s.Track),
		span.String("kubedojo.mode", s.Mode),
	}, attrs...)
	return m.tracer(ctx).Start(ctx, name, all...)
}

// errGrader marks a question whose checks could not all be evaluated.
var errGrader = errors.New("grader error")

// errorType names the kind of a failure in a few fixed words, so that a
// trace query can tell a refused request from a broken server.
func errorType(err error) string {
	switch {
	case errors.Is(err, errGrader):
		return checker.OutcomeGraderError
	case errors.Is(err, ErrSandboxGone):
		return "sandbox_gone"
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrInvalid):
		return "invalid"
	case errors.Is(err, ErrLimit), errors.Is(err, ErrTerminalLimit):
		return "limit"
	case errors.Is(err, ErrConflict):
		return "conflict"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	return "internal"
}

// end ends a span of session work and names the kind of its failure.
func end(sp span.Span, err error) {
	if err != nil {
		sp.Set(span.String("error.type", errorType(err)))
	}
	sp.End(err)
}

// graded describes a graded question on its span question.check.
func graded(res checker.Result) []span.Attr {
	failed := 0
	for _, c := range res.Checks {
		if !c.Passed {
			failed++
		}
	}
	return []span.Attr{
		span.Int("kubedojo.question.score", res.Score),
		span.Int("kubedojo.question.max", res.Max),
		span.Bool("kubedojo.question.passed", res.Passed),
		span.Int("kubedojo.checks.total", len(res.Checks)),
		span.Int("kubedojo.checks.failed", failed),
	}
}

// gradingError is the error of a question.check span. A question the learner
// did not solve is no error: the span says so in its attributes. Only checks
// that could not be evaluated are one.
func gradingError(res checker.Result) error {
	if n := res.GraderErrors(); n > 0 {
		return fmt.Errorf("%w: %d of %d checks could not be evaluated", errGrader, n, len(res.Checks))
	}
	return nil
}

// Why a command runs in a sandbox outside a check. It is the attribute
// kubedojo.exec.purpose of the span sandbox.exec.
const (
	ExecSetup      = "setup"      // setup.sh of a question
	ExecCleanup    = "cleanup"    // cleanup.sh before a question is set up again
	ExecHeal       = "heal"       // cleanup.sh of another question that left the environment broken
	ExecSolution   = "solution"   // a reference solution (kubedojo content test)
	ExecSnapshot   = "snapshot"   // the state of the sandbox, collected for the assistant
	ExecDiagnostic = "diagnostic" // anything else; the default of Manager.Exec
)

type execPurposeKey struct{}

// WithExecPurpose says why the commands run with Manager.Exec in ctx are run:
// one of the Exec constants. Without it, and for any other word, the purpose
// is ExecDiagnostic.
func WithExecPurpose(ctx context.Context, purpose string) context.Context {
	return context.WithValue(ctx, execPurposeKey{}, purpose)
}

func execPurpose(ctx context.Context) string {
	switch p, _ := ctx.Value(execPurposeKey{}).(string); p {
	case ExecSetup, ExecCleanup, ExecHeal, ExecSolution, ExecSnapshot:
		return p
	}
	return ExecDiagnostic
}

// tracedExec runs a script for a question as the span sandbox.exec. Checks do
// not come through here: a check is the span check.run and needs no second
// one. With strict, an exit code other than 0 is a failure of the span.
func (m *Manager) tracedExec(ctx context.Context, s *Session, q *catalog.Question, purpose, role, user, script string, strict bool) (sandbox.ExecResult, error) {
	target := role
	if target == "" {
		target = m.envSpec(s.Track, q.Environment).DefaultRole()
	}
	ctx, sp := m.span(ctx, "sandbox.exec", s,
		span.String("kubedojo.question", q.ID),
		span.String("kubedojo.exec.purpose", purpose),
		span.String("kubedojo.exec.target", target))
	res, err := m.exec(ctx, s, q, role, user, script)
	if err != nil {
		end(sp, err)
		return res, err
	}
	sp.Set(span.Int("process.exit.code", res.ExitCode))
	if strict && res.ExitCode != 0 {
		end(sp, fmt.Errorf("exit code %d", res.ExitCode))
		return res, nil
	}
	end(sp, nil)
	return res, nil
}

// envType is the type of an environment as spans name it.
func envType(spec sandbox.EnvSpec) string {
	if spec.Type == "" {
		return sandbox.EnvCluster
	}
	return spec.Type
}

// envNodes is the number of machines an environment has.
func envNodes(spec sandbox.EnvSpec) int {
	if spec.Type == sandbox.EnvContainer {
		return 1
	}
	return 1 + spec.Workers
}
