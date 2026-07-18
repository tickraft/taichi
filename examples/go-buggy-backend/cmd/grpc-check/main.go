// Command grpc-check is a taichi plugin skill that exercises the gRPC
// ProductService of the go-buggy-backend example.
//
// It is invoked by taichi with the standard plugin protocol (stdin → JSON,
// stdout ← JSON). It reads the gRPC target from config, calls GetProduct(1)
// and ListProducts, and asserts the returned price and product count match
// the expected values.
//
// Intentional bug detected: GetProduct(1) returns price=9.99 instead of 99.99.
//
// Build: go build -o bin/grpc-check ./cmd/grpc-check
// Run (via taichi): declared as `kind: plugin` with raw.command=./bin/grpc-check
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	productpb "github.com/tickraft/taichi/examples/go-buggy-backend/proto/gen/product"
)

// pluginInput mirrors taichi's plugin.Input struct.
type pluginInput struct {
	SkillName   string         `json:"skill_name"`
	ProjectName string         `json:"project_name"`
	BaseURL     string         `json:"base_url,omitempty"`
	ReportsDir  string         `json:"reports_dir,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
}

// pluginCase mirrors taichi's plugin.Case struct.
type pluginCase struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	Skipped    bool   `json:"skipped,omitempty"`
	Message    string `json:"message,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// pluginOutput mirrors taichi's plugin.Output struct.
type pluginOutput struct {
	Cases []pluginCase `json:"cases"`
	Error string       `json:"error,omitempty"`
}

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		emit(nil, fmt.Errorf("read stdin: %w", err))
		os.Exit(1)
	}

	var in pluginInput
	if err := json.Unmarshal(raw, &in); err != nil {
		emit(nil, fmt.Errorf("parse stdin json: %w", err))
		os.Exit(1)
	}

	target, _ := in.Config["grpc_target"].(string)
	if target == "" {
		target = "127.0.0.1:50051"
	}
	expectedPrice, _ := in.Config["expected_price"].(float64)
	if expectedPrice == 0 {
		expectedPrice = 99.99
	}
	expectedTotal, _ := in.Config["expected_total"].(float64)
	if expectedTotal == 0 {
		expectedTotal = 2
	}

	fmt.Fprintf(os.Stderr, "[grpc-check] target=%s expected_price=%.2f expected_total=%.0f\n", target, expectedPrice, expectedTotal)

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		emit(nil, fmt.Errorf("dial %s: %w", target, err))
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	client := productpb.NewProductServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cases []pluginCase

	// Case 1: GetProduct(1) — expect price 99.99, but server returns 9.99 (bug).
	caseStart := time.Now()
	p, err := client.GetProduct(ctx, &productpb.GetProductRequest{Id: 1})
	if err != nil {
		cases = append(cases, pluginCase{
			Name: "GetProduct_1", Passed: false, Error: err.Error(),
			Message: "GetProduct(1) returned an error", DurationMs: ms(time.Since(caseStart)),
		})
	} else if p.GetPrice() != expectedPrice {
		cases = append(cases, pluginCase{
			Name: "GetProduct_1", Passed: false,
			Message:    fmt.Sprintf("price mismatch: expected %.2f, got %.2f", expectedPrice, p.GetPrice()),
			DurationMs: ms(time.Since(caseStart)),
			Error:      fmt.Sprintf("wrong price: %.2f", p.GetPrice()),
		})
	} else {
		cases = append(cases, pluginCase{
			Name: "GetProduct_1", Passed: true,
			Message:    fmt.Sprintf("price ok: %.2f", p.GetPrice()),
			DurationMs: ms(time.Since(caseStart)),
		})
	}

	// Case 2: ListProducts — expect 2 items.
	caseStart = time.Now()
	list, err := client.ListProducts(ctx, &productpb.ListProductsRequest{Limit: 10})
	if err != nil {
		cases = append(cases, pluginCase{
			Name: "ListProducts", Passed: false, Error: err.Error(),
			Message: "ListProducts returned an error", DurationMs: ms(time.Since(caseStart)),
		})
	} else if int(list.GetTotal()) != int(expectedTotal) {
		cases = append(cases, pluginCase{
			Name: "ListProducts", Passed: false,
			Message:    fmt.Sprintf("total mismatch: expected %.0f, got %d", expectedTotal, list.GetTotal()),
			DurationMs: ms(time.Since(caseStart)),
			Error:      fmt.Sprintf("wrong total: %d", list.GetTotal()),
		})
	} else {
		cases = append(cases, pluginCase{
			Name: "ListProducts", Passed: true,
			Message:    fmt.Sprintf("total ok: %d", list.GetTotal()),
			DurationMs: ms(time.Since(caseStart)),
		})
	}

	// Case 3: GetProduct(2) — sanity check, should pass.
	caseStart = time.Now()
	p2, err := client.GetProduct(ctx, &productpb.GetProductRequest{Id: 2})
	if err != nil {
		cases = append(cases, pluginCase{
			Name: "GetProduct_2", Passed: false, Error: err.Error(),
			Message: "GetProduct(2) returned an error", DurationMs: ms(time.Since(caseStart)),
		})
	} else {
		cases = append(cases, pluginCase{
			Name: "GetProduct_2", Passed: true,
			Message:    fmt.Sprintf("product: %s @ %.2f", p2.GetName(), p2.GetPrice()),
			DurationMs: ms(time.Since(caseStart)),
		})
	}

	emit(cases, nil)
}

func ms(d time.Duration) int64 { return d.Milliseconds() }

func emit(cases []pluginCase, err error) {
	out := pluginOutput{Cases: cases}
	if err != nil {
		out.Error = err.Error()
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(out)
}
