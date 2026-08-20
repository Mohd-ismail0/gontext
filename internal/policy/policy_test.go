package policy_test

import (
	"context"
	"testing"

	"github.com/xsama/context-fabric/internal/policy"
	"github.com/xsama/context-fabric/internal/ports"
)

func TestPolicyPurposeAllowlist(t *testing.T) {
	p := policy.New()
	res, err := p.Evaluate(context.Background(), ports.PolicyEval{
		Purpose:        "support",
		Classification: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allow || res.ReasonCode != "policy_ok" {
		t.Fatalf("got %#v", res)
	}

	res, err = p.Evaluate(context.Background(), ports.PolicyEval{
		Purpose:        "espionage",
		Classification: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allow || res.ReasonCode != "purpose_not_allowed" {
		t.Fatalf("got %#v", res)
	}
}

func TestPolicyClassificationCeilingAndRedaction(t *testing.T) {
	p := policy.New()
	res, err := p.Evaluate(context.Background(), ports.PolicyEval{
		Purpose:        "ops",
		Classification: "confidential",
		RequestedLimit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allow {
		t.Fatalf("expected allow: %#v", res)
	}
	if res.RedactionProfile != "pii_mask" {
		t.Fatalf("redaction=%q", res.RedactionProfile)
	}
	if res.MaxResults != 5 {
		t.Fatalf("max=%d", res.MaxResults)
	}
	found := false
	for _, o := range res.Obligations {
		if o == "mask_pii" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing mask_pii obligation: %v", res.Obligations)
	}

	res, err = p.Evaluate(context.Background(), ports.PolicyEval{
		Purpose:        "ops",
		Classification: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allow || res.ReasonCode != "classification_ceiling" {
		t.Fatalf("got %#v", res)
	}
}

func TestPolicyNeverGrantsWithoutPurpose(t *testing.T) {
	p := policy.New()
	res, err := p.Evaluate(context.Background(), ports.PolicyEval{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allow {
		t.Fatal("empty purpose must not allow")
	}
}

func TestPolicyUnknownClassificationFailClosed(t *testing.T) {
	p := policy.New()
	res, err := p.Evaluate(context.Background(), ports.PolicyEval{
		Purpose:        "ops",
		Classification: "top_secret_xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Allow || res.ReasonCode != "classification_unknown" {
		t.Fatalf("unknown classification must deny: %#v", res)
	}
}
