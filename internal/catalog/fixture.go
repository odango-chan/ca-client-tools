//go:build providerfixture

// Acceptance-only trusted bindings. This file is compiled only with the
// explicit `providerfixture` build tag so a deployed Provider binary never
// contains a fixture operation, fixture service key or fixture source.
package catalog

import (
	"encoding/json"
	"fmt"
)

// AcceptanceFixtureServiceKey is the trusted service key the acceptance
// harness must configure through CA_PROVIDER_SERVICE_CONFIG.
const AcceptanceFixtureServiceKey = "acceptance-fixture-sbi"

const acceptanceEchoBodySchema = `{
  "type": "object",
  "properties": {
    "label": {"type": "string", "minLength": 1, "maxLength": 64},
    "number": {"type": "integer"},
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "integer", "minimum": 0},
          "tags": {"type": "array", "items": {"type": "string"}},
          "weight": {"type": "number"}
        },
        "required": ["id"],
        "additionalProperties": false
      }
    }
  },
  "required": ["label", "number", "items"],
  "additionalProperties": false
}`

const acceptanceEchoArgumentSchema = `{
  "type": "object",
  "properties": {
    "mode": {"type": "string"},
    "body": {
      "type": "object",
      "properties": {
        "label": {"type": "string", "minLength": 1, "maxLength": 64},
        "number": {"type": "integer"},
        "items": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "id": {"type": "integer", "minimum": 0},
              "tags": {"type": "array", "items": {"type": "string"}},
              "weight": {"type": "number"}
            },
            "required": ["id"],
            "additionalProperties": false
          }
        }
      },
      "required": ["label", "number", "items"],
      "additionalProperties": false
    }
  },
  "required": ["body"],
  "additionalProperties": false
}`

const acceptanceMutationBodySchema = `{
  "type": "object",
  "properties": {
    "value": {"type": "string"}
  },
  "required": ["value"],
  "additionalProperties": false
}`

const acceptanceMutationArgumentSchema = `{
  "type": "object",
  "properties": {
    "requestId": {"type": "string"},
    "body": {
      "type": "object",
      "properties": {
        "value": {"type": "string"}
      },
      "required": ["value"],
      "additionalProperties": false
    }
  },
  "required": ["requestId", "body"],
  "additionalProperties": false
}`

// InstallAcceptanceBindings appends the acceptance-only genericSBI bindings to
// the single loaded catalog. It is called only by the providerfixture build.
func InstallAcceptanceBindings(target *Catalog) error {
	if target == nil {
		return fmt.Errorf("catalog: fixture install requires a loaded catalog")
	}
	for _, binding := range acceptanceBindings() {
		if _, duplicate := target.index[binding.BackendOperationID]; duplicate {
			return fmt.Errorf("catalog: fixture binding %q already exists", binding.BackendOperationID)
		}
		target.Bindings = append(target.Bindings, binding)
		target.index[binding.BackendOperationID] = len(target.Bindings) - 1
	}
	return nil
}

func acceptanceBindings() []Binding {
	echo := Binding{
		BackendOperationID: "acceptance.generic.echo",
		Effect:             EffectRead,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceFixtureServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "echo",
		Method:             "POST",
		Path:               "/acceptance/v1/echo",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/json"},
		Generic: &GenericSpec{
			Method: "POST",
			Path:   "/acceptance/v1/echo",
			QueryParameters: []Argument{
				{Name: "mode", Type: "string", Required: false, Default: "ok"},
			},
			HeaderBindings: []HeaderBinding{
				{Name: "X-Acceptance-Trusted", Value: "fixture-7"},
			},
			Body: &GenericBody{
				Kind:          "json",
				MediaType:     "application/json",
				RequestSchema: json.RawMessage(acceptanceEchoBodySchema),
			},
			ArgumentSchema:     json.RawMessage(acceptanceEchoArgumentSchema),
			ResponseMediaTypes: []string{"application/json"},
		},
	}
	mutation := Binding{
		BackendOperationID: "acceptance.generic.mutation",
		Effect:             EffectMutation,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceFixtureServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "mutation",
		Method:             "POST",
		Path:               "/acceptance/v1/mutation",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/json"},
		Generic: &GenericSpec{
			Method: "POST",
			Path:   "/acceptance/v1/mutation",
			Body: &GenericBody{
				Kind:          "json",
				MediaType:     "application/json",
				RequestSchema: json.RawMessage(acceptanceMutationBodySchema),
			},
			ArgumentSchema:     json.RawMessage(acceptanceMutationArgumentSchema),
			ResponseMediaTypes: []string{"application/json"},
			Idempotency: &Idempotency{
				Mode:        "IDEMPOTENCY_KEY",
				KeyArgument: "requestId",
			},
		},
	}
	return []Binding{echo, mutation}
}
