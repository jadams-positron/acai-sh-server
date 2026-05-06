// Package specs is the domain context for spec rows, feature-branch refs, and
// feature-impl states.
package specs

import "time"

// Spec mirrors a row in the specs table.
type Spec struct {
	ID                 string
	ProductID          string
	BranchID           string
	Path               *string
	LastSeenCommit     string
	ParsedAt           time.Time
	FeatureName        string
	FeatureDescription *string
	FeatureVersion     string
	RawContent         *string
	Requirements       map[string]Requirement
	InsertedAt         time.Time
	UpdatedAt          time.Time
}

// Requirement is one entry in Spec.Requirements (keyed by ACID).
type Requirement struct {
	Requirement string
	Deprecated  bool
	Note        *string
	ReplacedBy  []string
}

// FeatureBranchRef mirrors a row in feature_branch_refs.
type FeatureBranchRef struct {
	ID          string
	BranchID    string
	FeatureName string
	Refs        map[string][]CodeRef // keyed by ACID
	Commit      string
	PushedAt    time.Time
}

// CodeRef is one ref entry under an ACID.
type CodeRef struct {
	Path   string
	IsTest bool
}

// FeatureImplState mirrors a row in feature_impl_states.
type FeatureImplState struct {
	ID               string
	ImplementationID string
	FeatureName      string
	States           map[string]ACIDState // keyed by ACID
	UpdatedAt        time.Time
}

// ACIDState is one entry under FeatureImplState.States.
type ACIDState struct {
	Status    *string
	Comment   *string
	UpdatedAt *time.Time
}

// Branch is a thin domain type for branches.
type Branch struct {
	ID             string
	TeamID         string
	RepoURI        string
	BranchName     string
	LastSeenCommit string
	UpdatedAt      time.Time
}
