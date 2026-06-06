package evolution

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type ParticipantKind string

const (
	ParticipantControlAgent  ParticipantKind = "control-agent"
	ParticipantHumanApprover ParticipantKind = "human-approver"
)

type Participant struct {
	ID   string
	Kind ParticipantKind
}

type ProposalKind string

const (
	ProposalSchema ProposalKind = "schema.proposal"
	ProposalPlugin ProposalKind = "plugin.proposal"
	ProposalSkill  ProposalKind = "skill.proposal"
	ProposalPolicy ProposalKind = "policy.proposal"
)

type Stage string

const (
	StageSubmitted           Stage = "submitted"
	StageValidated           Stage = "validated"
	StageBuilt               Stage = "built"
	StageFixtureTested       Stage = "fixture-tested"
	StageShadowed            Stage = "shadowed"
	StageAdversarialVerified Stage = "adversarial-verified"
	StageApproved            Stage = "approved"
	StagePromoted            Stage = "promoted"
	StageRolledBack          Stage = "rolled-back"
	StageRejected            Stage = "rejected"
)

type CapabilityRegistry struct {
	Allowed map[string]bool
}

func DefaultCapabilityRegistry() CapabilityRegistry {
	return CapabilityRegistry{Allowed: map[string]bool{
		"read_state_view": true,
	}}
}

type PluginSpec struct {
	ID           string
	Version      string
	Capabilities []string
	Handles      []string
	Emits        []string
}

type EventSchema struct {
	EventType      string
	Version        int
	RequiredFields []string
}

type PolicySpec struct {
	Grants []PolicyGrant
}

type PolicyGrant struct {
	ActorKind   string
	Resource    string
	DirectWrite bool
}

type SkillSpec struct {
	SkillID string
	Status  string
}

type EvolutionProposal struct {
	ID              string
	Kind            ProposalKind
	Stage           Stage
	Actor           string
	Plugin          *PluginSpec
	Schema          *EventSchema
	Policy          *PolicySpec
	Skill           *SkillSpec
	RejectionReason string
}

type Governance struct {
	capabilities CapabilityRegistry
	proposals    map[string]EvolutionProposal
	plugins      map[string]PluginSpec
	schemas      map[schemaKey]EventSchema
}

type schemaKey struct {
	eventType string
	version   int
}

func NewGovernance(capabilities CapabilityRegistry) *Governance {
	return &Governance{
		capabilities: capabilities,
		proposals:    map[string]EvolutionProposal{},
		plugins:      map[string]PluginSpec{},
		schemas:      map[schemaKey]EventSchema{},
	}
}

func (g *Governance) Submit(actor Participant, proposal EvolutionProposal) (EvolutionProposal, error) {
	if err := validateParticipant(actor); err != nil {
		return EvolutionProposal{}, err
	}
	if actor.Kind != ParticipantControlAgent && actor.Kind != ParticipantHumanApprover {
		return EvolutionProposal{}, fmt.Errorf("participant %q cannot submit evolution proposals", actor.Kind)
	}
	if _, exists := g.proposals[proposal.ID]; exists {
		return EvolutionProposal{}, fmt.Errorf("evolution proposal %q already exists", proposal.ID)
	}
	proposal.Stage = StageSubmitted
	proposal.Actor = actor.ID
	if err := g.validateProposal(proposal); err != nil {
		return EvolutionProposal{}, err
	}
	proposal = cloneProposal(proposal)
	g.proposals[proposal.ID] = proposal
	return cloneProposal(proposal), nil
}

func (g *Governance) Transition(actor Participant, id string, next Stage) error {
	if actor.Kind != ParticipantHumanApprover {
		return fmt.Errorf("%s cannot transition evolution proposals", actor.Kind)
	}
	current, ok := g.proposals[id]
	if !ok {
		return fmt.Errorf("evolution proposal %q not found", id)
	}
	if current.Stage == StageRejected || current.Stage == StagePromoted || current.Stage == StageRolledBack {
		return fmt.Errorf("evolution proposal %q is terminal in %s", id, current.Stage)
	}
	if next == StageSubmitted || next == StagePromoted || next == StageRolledBack || next == StageRejected {
		return fmt.Errorf("use submit, promote, rollback, or reject for stage %s", next)
	}
	if !stageAfter(current.Stage, next) {
		return fmt.Errorf("invalid evolution stage transition %s -> %s", current.Stage, next)
	}
	current.Stage = next
	g.proposals[id] = current
	return nil
}

func (g *Governance) Promote(actor Participant, id string) error {
	if actor.Kind != ParticipantHumanApprover {
		return fmt.Errorf("%s cannot promote evolution proposals", actor.Kind)
	}
	current, ok := g.proposals[id]
	if !ok {
		return fmt.Errorf("evolution proposal %q not found", id)
	}
	if current.Stage != StageApproved {
		return fmt.Errorf("evolution proposal %q must be approved before promotion; current stage is %s", id, current.Stage)
	}
	switch current.Kind {
	case ProposalPlugin:
		if current.Plugin == nil {
			return errors.New("plugin proposal missing plugin spec")
		}
		g.plugins[current.Plugin.ID] = clonePluginSpec(*current.Plugin)
	case ProposalSchema:
		if current.Schema == nil {
			return errors.New("schema proposal missing schema spec")
		}
		if err := g.RegisterSchema(*current.Schema); err != nil {
			return err
		}
	case ProposalSkill, ProposalPolicy:
		// These proposal kinds are recorded for governance, but applying them still goes
		// through their domain-specific reviewed path.
	default:
		return fmt.Errorf("unknown evolution proposal kind %q", current.Kind)
	}
	current.Stage = StagePromoted
	g.proposals[id] = current
	return nil
}

func (g *Governance) Reject(actor Participant, id, reason string) error {
	if actor.Kind != ParticipantHumanApprover {
		return fmt.Errorf("%s cannot reject evolution proposals", actor.Kind)
	}
	current, ok := g.proposals[id]
	if !ok {
		return fmt.Errorf("evolution proposal %q not found", id)
	}
	if current.Stage == StagePromoted || current.Stage == StageRolledBack {
		return fmt.Errorf("evolution proposal %q is already %s", id, current.Stage)
	}
	current.Stage = StageRejected
	current.RejectionReason = strings.TrimSpace(reason)
	g.proposals[id] = current
	return nil
}

func (g *Governance) RegisterPlugin(spec PluginSpec) {
	g.plugins[spec.ID] = clonePluginSpec(spec)
}

func (g *Governance) Plugin(id string) (PluginSpec, bool) {
	spec, ok := g.plugins[id]
	return clonePluginSpec(spec), ok
}

func (g *Governance) RegisterSchema(schema EventSchema) error {
	if err := validateSchema(schema); err != nil {
		return err
	}
	key := schemaKey{eventType: schema.EventType, version: schema.Version}
	if existing, ok := g.schemas[key]; ok && !sameSchema(existing, schema) {
		return fmt.Errorf("schema %s v%d already exists with different required fields", schema.EventType, schema.Version)
	}
	g.schemas[key] = cloneSchema(schema)
	return nil
}

func (g *Governance) validateProposal(proposal EvolutionProposal) error {
	if strings.TrimSpace(proposal.ID) == "" {
		return errors.New("evolution proposal id is required")
	}
	switch proposal.Kind {
	case ProposalPlugin:
		if proposal.Plugin == nil {
			return errors.New("plugin proposal requires plugin spec")
		}
		return g.validatePluginProposal(*proposal.Plugin)
	case ProposalSchema:
		if proposal.Schema == nil {
			return errors.New("schema proposal requires schema spec")
		}
		return g.RegisterSchemaDryRun(*proposal.Schema)
	case ProposalPolicy:
		if proposal.Policy == nil {
			return errors.New("policy proposal requires policy spec")
		}
		return validatePolicyProposal(*proposal.Policy)
	case ProposalSkill:
		if proposal.Skill == nil || strings.TrimSpace(proposal.Skill.SkillID) == "" {
			return errors.New("skill proposal requires skill_id")
		}
		return nil
	default:
		return fmt.Errorf("unknown evolution proposal kind %q", proposal.Kind)
	}
}

func (g *Governance) validatePluginProposal(spec PluginSpec) error {
	if strings.TrimSpace(spec.ID) == "" {
		return errors.New("plugin id is required")
	}
	if strings.TrimSpace(spec.Version) == "" {
		return errors.New("plugin version is required")
	}
	if len(spec.Handles) == 0 || len(spec.Emits) == 0 {
		return errors.New("plugin proposal requires handles and emits")
	}
	for _, cap := range spec.Capabilities {
		if !g.capabilities.Allowed[cap] {
			return fmt.Errorf("plugin proposal %q requests unsupported capability %q", spec.ID, cap)
		}
	}
	if active, ok := g.plugins[spec.ID]; ok {
		activeCaps := stringSet(active.Capabilities)
		for _, cap := range spec.Capabilities {
			if !activeCaps[cap] {
				return fmt.Errorf("plugin proposal %q widens capabilities with %q without explicit capability registry approval", spec.ID, cap)
			}
		}
	}
	return nil
}

func (g *Governance) RegisterSchemaDryRun(schema EventSchema) error {
	if err := validateSchema(schema); err != nil {
		return err
	}
	key := schemaKey{eventType: schema.EventType, version: schema.Version}
	if existing, ok := g.schemas[key]; ok && !sameSchema(existing, schema) {
		return fmt.Errorf("schema proposal would make %s v%d ambiguous", schema.EventType, schema.Version)
	}
	return nil
}

func validateSchema(schema EventSchema) error {
	if strings.TrimSpace(schema.EventType) == "" {
		return errors.New("event schema type is required")
	}
	if schema.Version <= 0 {
		return errors.New("event schema version must be positive")
	}
	if len(schema.RequiredFields) == 0 {
		return errors.New("event schema requires at least one required field")
	}
	return nil
}

func validatePolicyProposal(policy PolicySpec) error {
	for _, grant := range policy.Grants {
		if grant.DirectWrite && grant.ActorKind == "host-agent" {
			return errors.New("policy proposal cannot grant HostAgent direct canonical write authority")
		}
	}
	return nil
}

func validateParticipant(actor Participant) error {
	if strings.TrimSpace(actor.ID) == "" {
		return errors.New("participant id is required")
	}
	if actor.Kind == "" {
		return errors.New("participant kind is required")
	}
	return nil
}

func stageAfter(current, next Stage) bool {
	currentIndex, okCurrent := stageOrder[current]
	nextIndex, okNext := stageOrder[next]
	return okCurrent && okNext && nextIndex > currentIndex
}

var stageOrder = map[Stage]int{
	StageSubmitted:           0,
	StageValidated:           1,
	StageBuilt:               2,
	StageFixtureTested:       3,
	StageShadowed:            4,
	StageAdversarialVerified: 5,
	StageApproved:            6,
}

func sameSchema(a, b EventSchema) bool {
	return a.EventType == b.EventType && a.Version == b.Version && reflect.DeepEqual(sortedStrings(a.RequiredFields), sortedStrings(b.RequiredFields))
}

func cloneProposal(in EvolutionProposal) EvolutionProposal {
	out := in
	if in.Plugin != nil {
		plugin := clonePluginSpec(*in.Plugin)
		out.Plugin = &plugin
	}
	if in.Schema != nil {
		schema := cloneSchema(*in.Schema)
		out.Schema = &schema
	}
	if in.Policy != nil {
		policy := PolicySpec{Grants: append([]PolicyGrant(nil), in.Policy.Grants...)}
		out.Policy = &policy
	}
	if in.Skill != nil {
		skill := *in.Skill
		out.Skill = &skill
	}
	return out
}

func clonePluginSpec(in PluginSpec) PluginSpec {
	return PluginSpec{
		ID:           in.ID,
		Version:      in.Version,
		Capabilities: append([]string(nil), in.Capabilities...),
		Handles:      append([]string(nil), in.Handles...),
		Emits:        append([]string(nil), in.Emits...),
	}
}

func cloneSchema(in EventSchema) EventSchema {
	return EventSchema{EventType: in.EventType, Version: in.Version, RequiredFields: append([]string(nil), in.RequiredFields...)}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}
