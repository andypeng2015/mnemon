package server

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

// ActorKind classifies a channel principal by role. It is NOT a privilege path: the channel is
// the same for every principal; the role differs by binding, never by a privileged code path
// (D6). HostAgent pushes host observations; ControlAgent is an operator/control client.
type ActorKind string

const (
	KindHostAgent    ActorKind = "host-agent"
	KindControlAgent ActorKind = "control-agent"
)

// Transport names the wire a binding uses.
type Transport string

const (
	TransportLocal Transport = "local" // in-process / local socket, trusted header
	TransportHTTP  Transport = "http"  // loopback / network http
	TransportMTLS  Transport = "mtls"  // mutual-TLS authenticated
)

// Verb is a channel operation. The channel exposes observe (Ingest) + pull (PullProjection) +
// status; claim/finish (the work lane) are reserved for a later phase.
type Verb string

const (
	VerbObserve Verb = "observe"
	VerbPull    Verb = "pull"
	VerbStatus  Verb = "status"
)

// ChannelBinding is the manifest that scopes ONE principal's access to the channel (D6). The
// channel is the same for every binding; the binding — never a privileged code path — is what
// differs a HostAgent from a ControlAgent. The server still enforces the scope at runtime (S9);
// the binding makes the grant explicit and auditable.
type ChannelBinding struct {
	Principal            contract.ActorID       // the authenticated identity
	ActorKind            ActorKind              // role classification (not a privilege path)
	Transport            Transport              // wire
	Endpoint             string                 // base URL / socket path
	AllowedVerbs         []Verb                 // observe / pull / status
	AllowedObservedTypes []string               // observed event types this principal may Ingest ("" or "*" = any)
	SubscriptionScope    []contract.ResourceRef // the refs this principal may pull
	IdempotencyNamespace string                 // prefix isolating this principal's ExternalIDs (cross-principal dedup isolation)
}

// Validate checks the binding is well-formed: a principal, a known kind, at least one verb.
func (b ChannelBinding) Validate() error {
	if strings.TrimSpace(string(b.Principal)) == "" {
		return fmt.Errorf("channel binding requires a principal")
	}
	if b.ActorKind != KindHostAgent && b.ActorKind != KindControlAgent {
		return fmt.Errorf("channel binding actor_kind %q is not host-agent or control-agent", b.ActorKind)
	}
	if len(b.AllowedVerbs) == 0 {
		return fmt.Errorf("channel binding %q grants no verbs", b.Principal)
	}
	return nil
}

// Allows reports whether the binding grants verb.
func (b ChannelBinding) Allows(v Verb) bool {
	for _, av := range b.AllowedVerbs {
		if av == v {
			return true
		}
	}
	return false
}

// AllowsObservedType reports whether the binding permits Ingesting an observation of eventType.
// An empty AllowedObservedTypes (or a "*" entry) means any observed type.
func (b ChannelBinding) AllowsObservedType(eventType string) bool {
	if len(b.AllowedObservedTypes) == 0 {
		return true
	}
	for _, t := range b.AllowedObservedTypes {
		if t == "*" || t == eventType {
			return true
		}
	}
	return false
}

// HostAgentBinding and ControlAgentBinding are the two canonical bindings over the SAME channel —
// the role differs ONLY by the binding (zero new surface for the control agent, D6).
func HostAgentBinding(principal contract.ActorID, endpoint string, scope []contract.ResourceRef) ChannelBinding {
	return ChannelBinding{
		Principal: principal, ActorKind: KindHostAgent, Transport: TransportHTTP, Endpoint: endpoint,
		AllowedVerbs: []Verb{VerbObserve, VerbPull, VerbStatus}, SubscriptionScope: scope,
		IdempotencyNamespace: "host:" + string(principal),
	}
}

func ControlAgentBinding(principal contract.ActorID, endpoint string, scope []contract.ResourceRef) ChannelBinding {
	return ChannelBinding{
		Principal: principal, ActorKind: KindControlAgent, Transport: TransportHTTP, Endpoint: endpoint,
		AllowedVerbs: []Verb{VerbObserve, VerbPull, VerbStatus}, SubscriptionScope: scope,
		IdempotencyNamespace: "control:" + string(principal),
	}
}
