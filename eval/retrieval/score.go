// Package main scores a retrieval run against the labeled corpus.
// Usage: go run ./eval/retrieval -predictions predictions.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
)

type task struct {
	ID             string   `json:"id"`
	GoldResources  []string `json:"gold_resources"`
	ExpectAbstain  bool     `json:"expect_abstain"`
	Route          string   `json:"route"`
}

type prediction struct {
	ID            string   `json:"id"`
	ResourceIDs   []string `json:"resource_ids"`
	ToolCalls     int      `json:"tool_calls"`
	LatencyMS     float64  `json:"latency_ms"`
	AuthzChecks   int      `json:"authz_checks"`
	Abstained     bool     `json:"abstained"`
	TokensApprox  int      `json:"tokens_approx"`
}

func main() {
	corpPath := flag.String("corpus", "eval/retrieval/corpus.jsonl", "labeled corpus")
	predPath := flag.String("predictions", "", "predictions jsonl (required)")
	k := flag.Int("k", 5, "Recall@K")
	flag.Parse()
	if *predPath == "" {
		fmt.Fprintln(os.Stderr, "-predictions is required")
		os.Exit(2)
	}
	tasks, err := loadTasks(*corpPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	preds, err := loadPreds(*predPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var recallSum, mrrSum, citePrecSum float64
	var nRel, nCite, toolSum, authzSum, tokensSum, abstainOK, abstainN int
	var lat []float64
	scored := 0
	for _, t := range tasks {
		p, ok := preds[t.ID]
		if !ok {
			continue
		}
		scored++
		toolSum += p.ToolCalls
		authzSum += p.AuthzChecks
		tokensSum += p.TokensApprox
		lat = append(lat, p.LatencyMS)
		if t.ExpectAbstain {
			abstainN++
			if p.Abstained || len(p.ResourceIDs) == 0 {
				abstainOK++
			}
			continue
		}
		if len(t.GoldResources) == 0 {
			continue
		}
		nRel++
		gold := setOf(t.GoldResources)
		hits := 0
		firstRank := 0
		for i, id := range p.ResourceIDs {
			if i >= *k {
				break
			}
			if _, ok := gold[id]; ok {
				hits++
				if firstRank == 0 {
					firstRank = i + 1
				}
			}
		}
		recallSum += float64(hits) / float64(len(t.GoldResources))
		if firstRank > 0 {
			mrrSum += 1.0 / float64(firstRank)
		}
		for _, id := range p.ResourceIDs {
			nCite++
			if _, ok := gold[id]; ok {
				citePrecSum++
			}
		}
	}

	fmt.Printf("scored_tasks=%d\n", scored)
	if nRel > 0 {
		fmt.Printf("recall_at_%d=%.4f\n", *k, recallSum/float64(nRel))
		fmt.Printf("mrr=%.4f\n", mrrSum/float64(nRel))
	}
	if nCite > 0 {
		fmt.Printf("citation_precision=%.4f\n", citePrecSum/float64(nCite))
	}
	if scored > 0 {
		fmt.Printf("avg_tool_calls=%.2f\n", float64(toolSum)/float64(scored))
		fmt.Printf("avg_authz_checks=%.2f\n", float64(authzSum)/float64(scored))
		fmt.Printf("avg_tokens_approx=%.1f\n", float64(tokensSum)/float64(scored))
	}
	if abstainN > 0 {
		fmt.Printf("abstention_correctness=%.4f\n", float64(abstainOK)/float64(abstainN))
	}
	if len(lat) > 0 {
		sort.Float64s(lat)
		fmt.Printf("latency_p50_ms=%.1f\n", percentile(lat, 0.50))
		fmt.Printf("latency_p95_ms=%.1f\n", percentile(lat, 0.95))
	}
}

func loadTasks(path string) ([]task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []task
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var t task
		if err := json.Unmarshal(line, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, sc.Err()
}

func loadPreds(path string) (map[string]prediction, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]prediction{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var p prediction
		if err := json.Unmarshal(line, &p); err != nil {
			return nil, err
		}
		out[p.ID] = p
	}
	return out, sc.Err()
}

func setOf(ids []string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
