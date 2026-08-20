// Package mapping applies MappingSpec transforms to source intake payloads.
package mapping

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Spec is a versioned MappingSpec (contracts/jsonschema/mapping-spec.json).
type Spec struct {
	APIVersion     string         `json:"api_version"`
	ID             string         `json:"id"`
	Revision       string         `json:"revision"`
	OrganizationID string         `json:"organization_id"`
	SourceID       string         `json:"source_id"`
	Status         string         `json:"status"`
	Description    string         `json:"description,omitempty"`
	Mappings       FieldMappings  `json:"mappings"`
	Edges          []EdgeMapping  `json:"edges,omitempty"`
	Constraints    Constraints    `json:"constraints,omitempty"`
}

// EdgeMapping declares a knowledge-graph edge emitted on intake (ADR 0013).
// Edges never grant access; SyncAuthzParent only mirrors OpenFGA parent inheritance.
type EdgeMapping struct {
	Predicate       *Expr `json:"predicate"`
	From            *Expr `json:"from,omitempty"` // default: mapped resource_id
	To              *Expr `json:"to"`
	Confidence      *Expr `json:"confidence,omitempty"`
	EnsureEndpoints *bool `json:"ensure_endpoints,omitempty"` // default true
	SyncAuthzParent *bool `json:"sync_authz_parent,omitempty"` // default true when predicate=parent
}

// FieldMappings holds path/template expressions for canonical fields.
type FieldMappings struct {
	SourceExternalID   *Expr             `json:"source_external_id"`
	SourceRevision     *Expr             `json:"source_revision"`
	ContextSpaceID     *Expr             `json:"context_space_id"`
	ResourceID         *Expr             `json:"resource_id"`
	ResourceType       *Expr             `json:"resource_type"`
	BrandID            *Expr             `json:"brand_id,omitempty"`
	Timestamps         TimestampMappings `json:"timestamps"`
	ContentLocator     *Expr             `json:"content_locator"`
	ControlledTaxonomy map[string]*Expr  `json:"controlled_taxonomy,omitempty"`
	VisibilityRef      *Expr             `json:"visibility_ref,omitempty"`
	Classification     *Expr             `json:"classification,omitempty"`
	PurposeAllowlist   *Expr             `json:"purpose_allowlist,omitempty"`
	Trust              *Expr             `json:"trust,omitempty"`
	SourceAuthority    *Expr             `json:"source_authority,omitempty"`
	RetentionPolicyID  *Expr             `json:"retention_policy_id,omitempty"`
	Tags               *Expr             `json:"tags,omitempty"`
	OrganizationID     *Expr             `json:"organization_id,omitempty"`
	Title              *Expr             `json:"title,omitempty"`
	// ParentResourceID is a convenience mapping that emits predicate=parent
	// from the mapped resource to this target (same as an edges[] entry).
	ParentResourceID *Expr `json:"parent_resource_id,omitempty"`
}

// TimestampMappings maps occurred/observed timestamps.
type TimestampMappings struct {
	OccurredAt *Expr `json:"occurred_at"`
	ObservedAt *Expr `json:"observed_at"`
}

// Expr is a JSONPath, constant, or limited template over the payload.
type Expr struct {
	Expr      string `json:"expr"`
	Default   any    `json:"default,omitempty"`
	Transform string `json:"transform,omitempty"`
}

// Constraints are fixed safety rails (cannot be disabled by clients).
type Constraints struct {
	CannotMintOrganization     bool `json:"cannot_mint_organization"`
	CannotBroadenACL           bool `json:"cannot_broaden_acl"`
	AuthorityCeilingFromSource bool `json:"authority_ceiling_from_source"`
	ClientFieldsMayOnlyNarrow  bool `json:"client_fields_may_only_narrow"`
}

// SourceCeilings are authority limits from the source registration.
type SourceCeilings struct {
	OrganizationID            string
	TrustCeiling              string
	AuthorityCeiling          string
	ClassificationCeiling     string
	AllowedVisibilityRefs     []string
	AllowedRecordTypes        []string
}

// Canonical is the mapped intake fields ready for ledger write.
type Canonical struct {
	OrganizationID    string
	ResourceID        string
	ResourceType      string
	Title             string
	ContextSpaceID    string
	BrandID           string
	SourceExternalID  string
	SourceRevision    string
	ContentLocator    string
	Classification    string
	Trust             string
	Authority         string
	VisibilityRef     string
	PurposeAllowlist  string
	RetentionPolicyID string
	Tags              []string
	OccurredAt        time.Time
	ObservedAt        time.Time
	Attributes        map[string]string
	Edges             []CanonicalEdge
	DryRun            bool
}

// CanonicalEdge is a resolved knowledge-graph edge ready for ledger write.
type CanonicalEdge struct {
	FromID          string
	ToID            string
	Predicate       string
	Confidence      float64
	EnsureEndpoints bool
	SyncAuthzParent bool
	ToKind          string // hint when ensuring a stub endpoint
	ToTitle         string
	ToExternalID    string
}

// Options controls Apply behavior.
type Options struct {
	DryRun bool
}

// Apply evaluates MappingSpec against payload and enforces source ceilings.
func Apply(spec Spec, payload map[string]any, ceilings SourceCeilings, opts Options) (Canonical, error) {
	if ceilings.OrganizationID == "" {
		return Canonical{}, platform.ErrValidation("source organization_id required")
	}
	if spec.OrganizationID != "" && spec.OrganizationID != ceilings.OrganizationID {
		return Canonical{}, platform.ErrForbidden("mapping organization_id differs from source registration")
	}

	out := Canonical{
		OrganizationID: ceilings.OrganizationID,
		Attributes:     map[string]string{},
		DryRun:         opts.DryRun,
	}

	var err error
	if out.SourceExternalID, err = evalString(spec.Mappings.SourceExternalID, payload); err != nil {
		return Canonical{}, err
	}
	if out.SourceRevision, err = evalString(spec.Mappings.SourceRevision, payload); err != nil {
		return Canonical{}, err
	}
	if out.ContextSpaceID, err = evalString(spec.Mappings.ContextSpaceID, payload); err != nil {
		return Canonical{}, err
	}
	if out.ResourceID, err = evalString(spec.Mappings.ResourceID, payload); err != nil {
		return Canonical{}, err
	}
	if out.ResourceType, err = evalString(spec.Mappings.ResourceType, payload); err != nil {
		return Canonical{}, err
	}
	if out.BrandID, err = evalString(spec.Mappings.BrandID, payload); err != nil {
		return Canonical{}, err
	}
	if out.ContentLocator, err = evalString(spec.Mappings.ContentLocator, payload); err != nil {
		return Canonical{}, err
	}
	if out.Title, err = evalString(spec.Mappings.Title, payload); err != nil {
		return Canonical{}, err
	}
	if out.VisibilityRef, err = evalString(spec.Mappings.VisibilityRef, payload); err != nil {
		return Canonical{}, err
	}
	if out.PurposeAllowlist, err = evalString(spec.Mappings.PurposeAllowlist, payload); err != nil {
		return Canonical{}, err
	}
	if out.RetentionPolicyID, err = evalString(spec.Mappings.RetentionPolicyID, payload); err != nil {
		return Canonical{}, err
	}

	classRaw, err := evalString(spec.Mappings.Classification, payload)
	if err != nil {
		return Canonical{}, err
	}
	trustRaw, err := evalString(spec.Mappings.Trust, payload)
	if err != nil {
		return Canonical{}, err
	}
	authRaw, err := evalString(spec.Mappings.SourceAuthority, payload)
	if err != nil {
		return Canonical{}, err
	}

	if mappedOrg, err := evalString(spec.Mappings.OrganizationID, payload); err != nil {
		return Canonical{}, err
	} else if mappedOrg != "" && mappedOrg != ceilings.OrganizationID {
		return Canonical{}, platform.ErrForbidden("mapping cannot set organization_id different from source registration")
	}

	if out.OccurredAt, err = evalTime(spec.Mappings.Timestamps.OccurredAt, payload); err != nil {
		return Canonical{}, err
	}
	if out.ObservedAt, err = evalTime(spec.Mappings.Timestamps.ObservedAt, payload); err != nil {
		return Canonical{}, err
	}

	tagsRaw, err := evalString(spec.Mappings.Tags, payload)
	if err != nil {
		return Canonical{}, err
	}
	if tagsRaw != "" {
		out.Tags = splitCSV(tagsRaw)
	}
	for k, expr := range spec.Mappings.ControlledTaxonomy {
		v, err := evalString(expr, payload)
		if err != nil {
			return Canonical{}, err
		}
		if v != "" {
			out.Attributes[k] = v
			out.Tags = append(out.Tags, k+":"+v)
		}
	}

	out.Classification = firstNonEmpty(classRaw, ceilings.ClassificationCeiling, "internal")
	out.Trust = firstNonEmpty(trustRaw, "untrusted_external")
	out.Authority = firstNonEmpty(authRaw, "corroborating")

	if err := enforceCeilings(out, ceilings); err != nil {
		return Canonical{}, err
	}

	if out.ResourceID == "" {
		out.ResourceID = platform.NewResourceID()
	}
	if out.ResourceType == "" {
		out.ResourceType = "event"
	}
	if len(ceilings.AllowedRecordTypes) > 0 && !contains(ceilings.AllowedRecordTypes, out.ResourceType) {
		return Canonical{}, platform.ErrForbidden("resource_type not allowed by source registration")
	}

	out.Attributes["trust"] = out.Trust
	out.Attributes["authority"] = out.Authority
	out.Attributes["source_authority"] = out.Authority
	if out.VisibilityRef != "" {
		out.Attributes["visibility_ref"] = out.VisibilityRef
	}
	if out.PurposeAllowlist != "" {
		out.Attributes["purpose_allowlist"] = out.PurposeAllowlist
	}
	if out.ContextSpaceID != "" {
		out.Attributes["context_space_id"] = out.ContextSpaceID
	}
	if out.ContentLocator != "" {
		out.Attributes["content_locator"] = out.ContentLocator
	}

	edges, err := resolveEdges(spec, payload, out)
	if err != nil {
		return Canonical{}, err
	}
	out.Edges = edges

	return out, nil
}

func resolveEdges(spec Spec, payload map[string]any, out Canonical) ([]CanonicalEdge, error) {
	var edges []CanonicalEdge
	seen := map[string]bool{}

	add := func(e CanonicalEdge) {
		if e.FromID == "" || e.ToID == "" || e.Predicate == "" || e.FromID == e.ToID {
			return
		}
		key := e.FromID + "|" + e.Predicate + "|" + e.ToID
		if seen[key] {
			return
		}
		seen[key] = true
		if e.Confidence <= 0 {
			e.Confidence = 1
		}
		edges = append(edges, e)
	}

	// Convenience parent_resource_id mapping.
	if parentID, err := evalString(spec.Mappings.ParentResourceID, payload); err != nil {
		return nil, err
	} else if parentID != "" {
		add(CanonicalEdge{
			FromID:          out.ResourceID,
			ToID:            parentID,
			Predicate:       ports.EdgeParent,
			Confidence:      1,
			EnsureEndpoints: true,
			SyncAuthzParent: true,
			ToKind:          "container",
		})
	}

	for i, em := range spec.Edges {
		pred, err := evalString(em.Predicate, payload)
		if err != nil {
			return nil, fmt.Errorf("edges[%d].predicate: %w", i, err)
		}
		if pred == "" {
			pred = ports.EdgeParent
		}
		fromID, err := evalString(em.From, payload)
		if err != nil {
			return nil, fmt.Errorf("edges[%d].from: %w", i, err)
		}
		if fromID == "" {
			fromID = out.ResourceID
		}
		toID, err := evalString(em.To, payload)
		if err != nil {
			return nil, fmt.Errorf("edges[%d].to: %w", i, err)
		}
		if toID == "" {
			continue
		}
		conf := 1.0
		if em.Confidence != nil {
			raw, err := evalString(em.Confidence, payload)
			if err != nil {
				return nil, fmt.Errorf("edges[%d].confidence: %w", i, err)
			}
			if raw != "" {
				if f, err := strconv.ParseFloat(raw, 64); err == nil {
					conf = f
				}
			}
		}
		ensure := true
		if em.EnsureEndpoints != nil {
			ensure = *em.EnsureEndpoints
		}
		syncParent := strings.EqualFold(pred, ports.EdgeParent)
		if em.SyncAuthzParent != nil {
			syncParent = *em.SyncAuthzParent
		}
		add(CanonicalEdge{
			FromID:          fromID,
			ToID:            toID,
			Predicate:       strings.ToLower(pred),
			Confidence:      conf,
			EnsureEndpoints: ensure,
			SyncAuthzParent: syncParent,
		})
	}

	// Auto parent from visibility_ref when no explicit parent edge yet.
	if out.VisibilityRef != "" {
		hasParent := false
		for _, e := range edges {
			if e.Predicate == ports.EdgeParent && e.FromID == out.ResourceID {
				hasParent = true
				break
			}
		}
		if !hasParent {
			if pe, ok := edgeFromVisibilityRef(out.ResourceID, out.VisibilityRef); ok {
				add(pe)
			}
		}
	}

	return edges, nil
}

// edgeFromVisibilityRef derives a parent edge from refs like "resource:<id>#viewer"
// or "case:SUP-412#viewer" (stable uuid_v5 of the object key).
func edgeFromVisibilityRef(resourceID, visibilityRef string) (CanonicalEdge, bool) {
	ref := strings.TrimSpace(visibilityRef)
	if ref == "" {
		return CanonicalEdge{}, false
	}
	object := ref
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		object = ref[:i]
	}
	object = strings.TrimSpace(object)
	if object == "" {
		return CanonicalEdge{}, false
	}

	var parentID, kind, title, external string
	switch {
	case strings.HasPrefix(object, "resource:"):
		parentID = strings.TrimPrefix(object, "resource:")
		kind = "resource"
		title = parentID
		external = object
	default:
		// kind:external_id → deterministic stub resource id
		kind, external = object, object
		if i := strings.IndexByte(object, ':'); i > 0 {
			kind = object[:i]
		}
		parentID = uuidV5Hex(object)
		title = object
	}
	if parentID == "" || parentID == resourceID {
		return CanonicalEdge{}, false
	}
	return CanonicalEdge{
		FromID:          resourceID,
		ToID:            parentID,
		Predicate:       ports.EdgeParent,
		Confidence:      1,
		EnsureEndpoints: true,
		SyncAuthzParent: true,
		ToKind:          kind,
		ToTitle:         title,
		ToExternalID:    external,
	}, true
}

func uuidV5Hex(name string) string {
	// Same namespace as MappingSpec transform "uuid_v5".
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

// ToRecord builds a ports.Record from mapped canonical fields.
func (c Canonical) ToRecord(orgID, sourceSystem string) ports.Record {
	now := time.Now().UTC()
	return ports.Record{
		ResourceID:     c.ResourceID,
		OrgID:          orgID,
		Kind:           c.ResourceType,
		Title:          firstNonEmpty(c.Title, c.SourceExternalID),
		Classification: c.Classification,
		Labels:         append([]string{}, c.Tags...),
		SourceSystem:   sourceSystem,
		ExternalID:     c.SourceExternalID,
		State:          "ACCEPTED",
		Attributes:     copyAttrs(c.Attributes),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func enforceCeilings(out Canonical, ceilings SourceCeilings) error {
	if ceilings.TrustCeiling != "" {
		outRank, ceilRank := RankTrust(out.Trust), RankTrust(ceilings.TrustCeiling)
		if outRank < 0 || ceilRank < 0 || outRank > ceilRank {
			return platform.ErrForbidden("mapping raises trust above source ceiling")
		}
	}
	if ceilings.AuthorityCeiling != "" {
		outRank, ceilRank := RankAuthority(out.Authority), RankAuthority(ceilings.AuthorityCeiling)
		if outRank < 0 || ceilRank < 0 || outRank > ceilRank {
			return platform.ErrForbidden("mapping raises authority above source ceiling")
		}
	}
	if ceilings.ClassificationCeiling != "" {
		outRank, ceilRank := RankClassification(out.Classification), RankClassification(ceilings.ClassificationCeiling)
		if outRank < 0 || ceilRank < 0 || outRank > ceilRank {
			return platform.ErrForbidden("mapping raises classification above source ceiling")
		}
	}
	if out.VisibilityRef != "" && len(ceilings.AllowedVisibilityRefs) > 0 {
		if !contains(ceilings.AllowedVisibilityRefs, out.VisibilityRef) {
			return platform.ErrForbidden("mapping broadens ACL via visibility_ref outside source policy")
		}
	}
	return nil
}

// RankTrust orders trust tiers; higher means more trusted. Unknown values return -1.
func RankTrust(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "generated":
		return 1
	case "untrusted_external":
		return 2
	case "trusted_internal":
		return 3
	case "trusted_system":
		return 4
	default:
		return -1
	}
}

// RankAuthority orders authority; higher means more authoritative. Unknown values return -1.
func RankAuthority(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "inferred":
		return 1
	case "user_claim":
		return 2
	case "corroborating":
		return 3
	case "source_of_truth":
		return 4
	default:
		return -1
	}
}

// RankClassification orders sensitivity; higher means more sensitive. Unknown values return -1.
func RankClassification(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "public":
		return 1
	case "internal":
		return 2
	case "confidential":
		return 3
	case "restricted":
		return 4
	default:
		return -1
	}
}

func evalString(expr *Expr, payload map[string]any) (string, error) {
	if expr == nil || strings.TrimSpace(expr.Expr) == "" {
		if expr != nil && expr.Default != nil {
			return stringify(expr.Default), nil
		}
		return "", nil
	}
	raw, err := resolve(expr.Expr, payload)
	if err != nil {
		return "", err
	}
	if raw == nil || raw == "" {
		if expr.Default != nil {
			raw = expr.Default
		} else {
			return "", nil
		}
	}
	s := stringify(raw)
	return applyTransform(s, expr.Transform), nil
}

func evalTime(expr *Expr, payload map[string]any) (time.Time, error) {
	s, err := evalString(expr, payload)
	if err != nil {
		return time.Time{}, err
	}
	if s == "" {
		return time.Now().UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(n, 0).UTC(), nil
	}
	return time.Time{}, platform.ErrValidation("invalid timestamp: " + s)
}

func resolve(expr string, payload map[string]any) (any, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}
	if (strings.HasPrefix(expr, `"`) && strings.HasSuffix(expr, `"`)) ||
		(strings.HasPrefix(expr, `'`) && strings.HasSuffix(expr, `'`)) {
		return expr[1 : len(expr)-1], nil
	}
	if strings.HasPrefix(expr, "$.") || strings.HasPrefix(expr, "$[") {
		return dig(payload, strings.TrimPrefix(expr, "$")), nil
	}
	if strings.HasPrefix(expr, ".") {
		return dig(payload, expr), nil
	}
	// Bare path like data.id
	if strings.Contains(expr, ".") && !strings.Contains(expr, " ") {
		return dig(payload, "."+expr), nil
	}
	return expr, nil
}

func dig(root map[string]any, path string) any {
	path = strings.TrimPrefix(path, ".")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return root
	}
	parts := strings.Split(path, ".")
	var cur any = root
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// support key[0]
		key := p
		idx := -1
		if i := strings.Index(p, "["); i >= 0 && strings.HasSuffix(p, "]") {
			key = p[:i]
			n, _ := strconv.Atoi(p[i+1 : len(p)-1])
			idx = n
		}
		switch node := cur.(type) {
		case map[string]any:
			if key == "" {
				cur = node
			} else {
				cur = node[key]
			}
		case map[string]string:
			cur = node[key]
		default:
			return nil
		}
		if idx >= 0 {
			arr, ok := cur.([]any)
			if !ok || idx >= len(arr) {
				return nil
			}
			cur = arr[idx]
		}
	}
	return cur
}

func applyTransform(s, transform string) string {
	switch strings.ToLower(strings.TrimSpace(transform)) {
	case "", "identity":
		return s
	case "lowercase":
		return strings.ToLower(s)
	case "trim":
		return strings.TrimSpace(s)
	case "sha256_hex":
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	case "uuid_v5":
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(s)).String()
	default:
		return s
	}
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, stringify(x))
		}
		return strings.Join(parts, ",")
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func copyAttrs(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ParseSpec unmarshals MappingSpec JSON/YAML-compatible JSON.
func ParseSpec(raw []byte) (Spec, error) {
	var s Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		return Spec{}, platform.ErrValidation("invalid mapping spec: " + err.Error())
	}
	return s, nil
}

// SpecFromPorts builds a minimal Spec from ports.MappingSpec rules map.
func SpecFromPorts(m ports.MappingSpec) Spec {
	mk := func(key string) *Expr {
		if m.Rules == nil {
			return nil
		}
		if v, ok := m.Rules[key]; ok && v != "" {
			return &Expr{Expr: v}
		}
		return nil
	}
	return Spec{
		ID:             m.ID,
		Revision:       m.Version,
		OrganizationID: m.OrgID,
		SourceID:       m.SourceID,
		Status:         "active",
		Mappings: FieldMappings{
			SourceExternalID: mk("source_external_id"),
			SourceRevision:   mk("source_revision"),
			ContextSpaceID:   mk("context_space_id"),
			ResourceID:       mk("resource_id"),
			ResourceType:     mk("resource_type"),
			ContentLocator:   mk("content_locator"),
			Classification:   mk("classification"),
			Trust:            mk("trust"),
			SourceAuthority:  mk("source_authority"),
			VisibilityRef:    mk("visibility_ref"),
			Title:            mk("title"),
			OrganizationID:   mk("organization_id"),
			ParentResourceID: mk("parent_resource_id"),
			Timestamps: TimestampMappings{
				OccurredAt: mk("occurred_at"),
				ObservedAt: mk("observed_at"),
			},
		},
	}
}
