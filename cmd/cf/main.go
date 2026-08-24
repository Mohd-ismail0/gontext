package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/conformance"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "doctor":
		err = cmdDoctor()
	case "tenant":
		err = cmdTenant(args)
	case "agent":
		err = cmdAgent(args)
	case "source":
		err = cmdSource(args)
	case "diagnose":
		err = cmdDiagnose(args)
	case "ops":
		err = cmdOps(args)
	case "conformance":
		err = cmdConformance(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cf %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `cf — Context Fabric operator CLI

Usage:
  cf doctor
  cf tenant provision|verify --org <id> [--name <name>]
  cf agent create|rotate|revoke --org <id> [--agent <id>] [--credential <id>]
  cf source register|verify --org <id> [--source <id>] [--system <name>]
  cf diagnose decision --org <id> --audit-id <id>
  cf ops lag|support-bundle --org <id>
  cf conformance run [--suite <path>] [--filter <case-id-substr>]

Environment:
  CONTEXT_FABRIC_URL   base URL (default http://127.0.0.1:8080)
  CONTEXT_FABRIC_TOKEN bearer token (e.g. local:<org>:<sub>:<role>)
  CONTEXT_FABRIC_CONFORMANCE_SUITE  default suite.yaml path
`)
}

func cmdConformance(args []string) error {
	if len(args) < 1 || args[0] != "run" {
		return fmt.Errorf("usage: cf conformance run [--suite <path>] [--filter <id>]")
	}
	suite := flagVal(args, "--suite")
	filter := flagVal(args, "--filter")
	rep, err := conformance.Run(context.Background(), conformance.RunOptions{
		SuitePath: suite,
		Filter:    filter,
	})
	if err != nil {
		return err
	}
	var failed, skipped, passed int
	for _, r := range rep.Results {
		switch {
		case r.Skipped:
			skipped++
			fmt.Printf("SKIP  %s — %s\n", r.ID, r.Detail)
		case r.Passed:
			passed++
			fmt.Printf("PASS  %s\n", r.ID)
		default:
			failed++
			fmt.Printf("FAIL  %s — %s\n", r.ID, r.Error)
		}
	}
	fmt.Printf("\nconformance %s: %d passed, %d failed, %d skipped\n", rep.SuiteID, passed, failed, skipped)
	if failed > 0 || !rep.Passed() {
		return fmt.Errorf("%d case(s) failed", failed)
	}
	return nil
}

func baseURL() string {
	u := strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_URL"))
	if u == "" {
		return "http://127.0.0.1:8080"
	}
	return strings.TrimRight(u, "/")
}

func token() string {
	return strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_TOKEN"))
}

func cmdDoctor() error {
	url := baseURL() + "/health/ready"
	resp, err := http.Get(url)
	if err != nil {
		if memoryHint() {
			fmt.Println("doctor: server unreachable; memory/demo mode tip:")
			fmt.Println("  CONTEXT_FABRIC_MEMORY=1 context-fabric serve")
			fmt.Println("  export CONTEXT_FABRIC_URL=http://127.0.0.1:8080")
			fmt.Println("  export CONTEXT_FABRIC_TOKEN=local:org1:alice:admin")
			return nil
		}
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("doctor: %s -> %s\n%s\n", url, resp.Status, string(body))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("health check failed")
	}
	return nil
}

func memoryHint() bool {
	return strings.EqualFold(os.Getenv("CONTEXT_FABRIC_MEMORY"), "1") ||
		strings.EqualFold(os.Getenv("CONTEXT_FABRIC_MEMORY"), "true") ||
		os.Getenv("CONTEXT_FABRIC_URL") == ""
}

func cmdTenant(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cf tenant provision|verify --org <id>")
	}
	sub := args[0]
	org := flagVal(args, "--org")
	name := flagVal(args, "--name")
	if org == "" {
		return fmt.Errorf("--org required")
	}
	if name == "" {
		name = org
	}
	switch sub {
	case "provision":
		return doJSON("POST", "/v1/organizations/"+org+"/bootstrap", map[string]any{"name": name})
	case "verify":
		return doJSON("GET", "/v1/organizations/"+org+"/status", nil)
	default:
		return fmt.Errorf("unknown tenant subcommand %q", sub)
	}
}

func cmdAgent(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cf agent create|rotate|revoke --org <id>")
	}
	sub := args[0]
	org := flagVal(args, "--org")
	agentID := flagVal(args, "--agent")
	credID := flagVal(args, "--credential")
	if org == "" {
		return fmt.Errorf("--org required")
	}
	switch sub {
	case "create":
		body := map[string]any{"agent_id": agentID, "owner_id": flagVal(args, "--owner")}
		return doJSON("POST", "/v1/organizations/"+org+"/agents", body)
	case "rotate":
		if agentID == "" {
			return fmt.Errorf("--agent required")
		}
		return doJSON("POST", "/v1/organizations/"+org+"/agents/"+agentID+"/credentials:rotate", map[string]any{})
	case "revoke":
		if credID == "" {
			return fmt.Errorf("--credential required")
		}
		return doJSON("POST", "/v1/organizations/"+org+"/agents/credentials/"+credID+":revoke", map[string]any{})
	default:
		return fmt.Errorf("unknown agent subcommand %q", sub)
	}
}

func cmdSource(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cf source register|verify --org <id>")
	}
	sub := args[0]
	org := flagVal(args, "--org")
	if org == "" {
		return fmt.Errorf("--org required")
	}
	switch sub {
	case "register":
		body := map[string]any{
			"source_id":    flagVal(args, "--source"),
			"system":       firstNonEmpty(flagVal(args, "--system"), "generic"),
			"display_name": flagVal(args, "--name"),
			"trust_tier":   firstNonEmpty(flagVal(args, "--trust-tier"), "verified"),
			"enabled":      true,
		}
		return doJSON("POST", "/v1/organizations/"+org+"/context/sources", body)
	case "verify":
		src := flagVal(args, "--source")
		if src == "" {
			return fmt.Errorf("--source required")
		}
		return doJSON("POST", "/v1/organizations/"+org+"/context/sources/"+src+":verify", map[string]any{})
	default:
		return fmt.Errorf("unknown source subcommand %q", sub)
	}
}

func cmdDiagnose(args []string) error {
	if len(args) < 1 || args[0] != "decision" {
		return fmt.Errorf("usage: cf diagnose decision --org <id> --audit-id <id>")
	}
	org := flagVal(args, "--org")
	auditID := flagVal(args, "--audit-id")
	if org == "" || auditID == "" {
		return fmt.Errorf("--org and --audit-id required")
	}
	return doJSON("GET", "/v1/organizations/"+org+"/context/diagnose/decision/"+auditID, nil)
}

func cmdOps(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cf ops lag|support-bundle --org <id>")
	}
	sub := args[0]
	org := flagVal(args, "--org")
	if org == "" {
		return fmt.Errorf("--org required")
	}
	switch sub {
	case "lag":
		return doJSON("GET", "/v1/organizations/"+org+"/ops/lag", nil)
	case "support-bundle":
		return doJSON("POST", "/v1/organizations/"+org+"/ops/support-bundle", map[string]any{})
	default:
		return fmt.Errorf("unknown ops subcommand %q", sub)
	}
}

func doJSON(method, path string, body any) error {
	tok := token()
	if tok == "" {
		fmt.Println("No CONTEXT_FABRIC_TOKEN set.")
		fmt.Println("Demo/memory instructions:")
		fmt.Println("  export CONTEXT_FABRIC_TOKEN=local:<org>:<sub>:<role>")
		fmt.Println("  export CONTEXT_FABRIC_URL=http://127.0.0.1:8080")
		return fmt.Errorf("missing token")
	}
	var rdr io.Reader
	if body != nil && method != "GET" {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL()+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s %s -> %s\n%s\n", method, path, resp.Status, string(out))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("request failed")
	}
	return nil
}

func flagVal(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
