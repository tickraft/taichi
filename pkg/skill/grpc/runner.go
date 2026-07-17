package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	"github.com/tickraft/taichi/pkg/skill"
)

// runCase dispatches a single gRPC case to its executor and returns a message + error.
// caseStart marks the beginning of the case for latency assertion.
func (s *Skill) runCase(ctx *skill.SkillContext, c grpcCase, caseStart time.Time) (string, error) {
	if c.Target == "" {
		return "", fmt.Errorf("target is empty (set skill.target or case.target to host:port)")
	}
	insecureMode := s.insecure
	if c.Insecure != nil {
		insecureMode = *c.Insecure
	}

	var (
		msg string
		err error
	)
	switch c.Type {
	case CaseHealth:
		msg, err = s.runHealth(ctx, c, insecureMode)
	case CaseDial:
		msg, err = s.runDial(ctx, c, insecureMode)
	case CaseReflect:
		msg, err = s.runReflect(ctx, c, insecureMode)
	default:
		return "", fmt.Errorf("unknown gRPC case type %q (want health|dial|reflect)", c.Type)
	}
	if err != nil {
		return msg, err
	}
	// Assert latency after a successful case.
	if err := assertLatency(ctx, c, caseStart); err != nil {
		return msg, err
	}
	return msg, nil
}

// dial establishes a gRPC connection to target with the given security mode.
// The caller is responsible for closing the returned connection.
func dial(target string, insecureConn bool, timeout time.Duration) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(timeout),
	}
	if insecureConn {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return grpc.Dial(target, opts...)
}

// runHealth performs a grpc.health.v1 Health/Check call and asserts the serving status.
func (s *Skill) runHealth(ctx *skill.SkillContext, c grpcCase, insecureConn bool) (string, error) {
	conn, err := dial(c.Target, insecureConn, s.timeout)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", c.Target, err)
	}
	defer func() { _ = conn.Close() }()

	client := grpc_health_v1.NewHealthClient(conn)
	reqCtx, cancel := context.WithTimeout(ctx.Ctx, s.timeout)
	defer cancel()

	expected := c.ExpectedStatus
	if expected == "" {
		expected = grpc_health_v1.HealthCheckResponse_SERVING.String()
	}

	resp, err := client.Check(reqCtx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return "", fmt.Errorf("health check %s: %w", c.Target, err)
	}
	actual := resp.Status.String()
	if actual != expected {
		return "", fmt.Errorf("health status %s: expected %s, got %s", c.Target, expected, actual)
	}
	return fmt.Sprintf("health %s: %s", c.Target, actual), nil
}

// runDial verifies that a connection can be established to the target.
func (s *Skill) runDial(ctx *skill.SkillContext, c grpcCase, insecureConn bool) (string, error) {
	conn, err := dial(c.Target, insecureConn, s.timeout)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", c.Target, err)
	}
	defer func() { _ = conn.Close() }()
	return fmt.Sprintf("dial %s: connected", c.Target), nil
}

// runReflect queries server reflection for the list of exposed services and asserts
// that all expected service names are present.
func (s *Skill) runReflect(ctx *skill.SkillContext, c grpcCase, insecureConn bool) (string, error) {
	conn, err := dial(c.Target, insecureConn, s.timeout)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", c.Target, err)
	}
	defer func() { _ = conn.Close() }()

	client := reflectionpb.NewServerReflectionClient(conn)
	reqCtx, cancel := context.WithTimeout(ctx.Ctx, s.timeout)
	defer cancel()

	stream, err := client.ServerReflectionInfo(reqCtx)
	if err != nil {
		return "", fmt.Errorf("open reflection stream %s: %w", c.Target, err)
	}
	// Request the list of all services.
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{},
	}); err != nil {
		return "", fmt.Errorf("send reflection request %s: %w", c.Target, err)
	}
	resp, err := stream.Recv()
	if err != nil {
		// gRPC status codes other than OK are failures; Unimplemented means the server
		// does not expose reflection, which is a legitimate test failure for this case.
		if st, ok := status.FromError(err); ok {
			return "", fmt.Errorf("reflection not available on %s: %s", c.Target, st.Code().String())
		}
		return "", fmt.Errorf("recv reflection response %s: %w", c.Target, err)
	}
	_ = stream.CloseSend()

	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		return "", fmt.Errorf("reflection %s: empty ListServicesResponse", c.Target)
	}

	// Collect exposed service names.
	exposed := make(map[string]struct{}, len(listResp.Service))
	for _, svc := range listResp.Service {
		exposed[svc.Name] = struct{}{}
	}

	// Assert all expected services are present.
	if len(c.ExpectedServices) > 0 {
		missing := make([]string, 0)
		for _, want := range c.ExpectedServices {
			if _, ok := exposed[want]; !ok {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			return "", fmt.Errorf("reflection %s: missing services: %s", c.Target, strings.Join(missing, ", "))
		}
	}
	return fmt.Sprintf("reflect %s: %d services exposed", c.Target, len(exposed)), nil
}

// assertLatency parses the case MaxLatency and asserts the elapsed time since caseStart
// fits within it using the shared assertion engine. Empty MaxLatency skips the assertion.
func assertLatency(ctx *skill.SkillContext, c grpcCase, caseStart time.Time) error {
	if c.MaxLatency == "" {
		return nil
	}
	max, err := time.ParseDuration(c.MaxLatency)
	if err != nil {
		return fmt.Errorf("invalid max_latency %q: %w", c.MaxLatency, err)
	}
	rt := ctx.Asserts.AssertResponseTime(time.Since(caseStart), max)
	if !rt.Passed {
		return fmt.Errorf("%s", rt.Message)
	}
	return nil
}
