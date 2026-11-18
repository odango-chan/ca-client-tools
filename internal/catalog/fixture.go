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

// AcceptanceArtifactServiceKey is the trusted service key of the P1 Appendix A
// artifact acceptance bindings. Real production bindings never use it.
const AcceptanceArtifactServiceKey = "acceptance-artifact"

// acceptanceReferenceSchema is the closed Host Artifact Store reference value
// (contract §2) as a catalog argument-schema fragment. It is used only by the
// providerfixture build.
const acceptanceReferenceSchema = `{
  "type": "object",
  "properties": {
    "artifactRef": {"type": "string", "pattern": "^art_[A-Za-z0-9_-]{43}$"}
  },
  "required": ["artifactRef"],
  "additionalProperties": false
}`

const acceptanceArtifactBinaryUpSchema = `{
  "type": "object",
  "properties": {
    "body": {
      "type": "object",
      "properties": {
        "artifactRef": {"type": "string", "pattern": "^art_[A-Za-z0-9_-]{43}$"}
      },
      "required": ["artifactRef"],
      "additionalProperties": false
    }
  },
  "required": ["body"],
  "additionalProperties": false
}`

const acceptanceArtifactMultipartBodySchema = `{
  "type": "object",
  "properties": {
    "requestId": {"type": "string", "minLength": 1, "maxLength": 128},
    "messages": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "n1MessageContent": {
            "type": "object",
            "properties": {"contentId": {"type": "string"}},
            "required": ["contentId"],
            "additionalProperties": false
          },
          "n2MessageContent": {
            "type": "object",
            "properties": {"contentId": {"type": "string"}},
            "required": ["contentId"],
            "additionalProperties": false
          }
        },
        "additionalProperties": false
      }
    }
  },
  "required": ["requestId", "messages"],
  "additionalProperties": false
}`

const acceptanceArtifactMultipartArgumentSchema = `{
  "type": "object",
  "properties": {
    "body": {
      "type": "object",
      "properties": {
        "requestId": {"type": "string", "minLength": 1, "maxLength": 128},
        "messages": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "n1MessageContent": {
                "type": "object",
                "properties": {"contentId": {"type": "string"}},
                "required": ["contentId"],
                "additionalProperties": false
              },
              "n2MessageContent": {
                "type": "object",
                "properties": {"contentId": {"type": "string"}},
                "required": ["contentId"],
                "additionalProperties": false
              }
            },
            "additionalProperties": false
          }
        }
      },
      "required": ["requestId", "messages"],
      "additionalProperties": false
    },
    "payloads": {
      "type": "object",
      "properties": {
        "n1Message": {
          "type": "object",
          "properties": {
            "artifactRef": {"type": "string", "pattern": "^art_[A-Za-z0-9_-]{43}$"}
          },
          "required": ["artifactRef"],
          "additionalProperties": false
        },
        "n2Message": {
          "type": "object",
          "properties": {
            "artifactRef": {"type": "string", "pattern": "^art_[A-Za-z0-9_-]{43}$"}
          },
          "required": ["artifactRef"],
          "additionalProperties": false
        }
      },
      "required": ["n1Message"],
      "additionalProperties": false
    }
  },
  "required": ["body", "payloads"],
  "additionalProperties": false
}`

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

const acceptanceEmptyArgumentSchema = `{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}`

const acceptanceBinaryResponseSchema = `{"type": "object"}`

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
	// Artifact-bearing fixtures model the P1 response shapes (single binary body
	// and multipart/related root plus attachment parts). P0 must reject them
	// before any remote I/O; the executor keeps its independent decoding
	// capability for the later Host Artifact Store integration.
	binary := Binding{
		BackendOperationID: "acceptance.generic.binary",
		Effect:             EffectRead,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceFixtureServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "binary",
		Method:             "GET",
		Path:               "/acceptance/v1/binary",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/octet-stream"},
		Generic: &GenericSpec{
			Method:             "GET",
			Path:               "/acceptance/v1/binary",
			ArgumentSchema:     json.RawMessage(acceptanceEmptyArgumentSchema),
			ResponseMediaTypes: []string{"application/octet-stream"},
		},
	}
	multipart := Binding{
		BackendOperationID: "acceptance.generic.multipart",
		Effect:             EffectRead,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceFixtureServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "multipart",
		Method:             "GET",
		Path:               "/acceptance/v1/multipart",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"multipart/related"},
		Generic: &GenericSpec{
			Method:             "GET",
			Path:               "/acceptance/v1/multipart",
			ArgumentSchema:     json.RawMessage(acceptanceEmptyArgumentSchema),
			ResponseMediaTypes: []string{"multipart/related"},
			ResponseSchema:     json.RawMessage(acceptanceBinaryResponseSchema),
			ResponseParts: []GenericPart{{
				Name:             "n2Message",
				ContentType:      "application/vnd.3gpp.ngap",
				JSONPointerField: "/n2MessageContent",
				Required:         true,
			}},
		},
	}
	// P1 Appendix A acceptance-only artifact bindings. They are exercised only
	// by the providerfixture build against a deterministic loopback upstream and
	// a Host Artifact Store channel test double; the production catalog never
	// contains them and its digest is unchanged.
	artifactBinaryUp := Binding{
		BackendOperationID: "acceptance.artifact.binary.up",
		Effect:             EffectRead,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceArtifactServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "artifactBinaryUp",
		Method:             "POST",
		Path:               "/acceptance/artifact/binary-up",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/json"},
		Generic: &GenericSpec{
			Method: "POST",
			Path:   "/acceptance/artifact/binary-up",
			Body: &GenericBody{
				Kind:      "binary",
				MediaType: "application/octet-stream",
			},
			ArgumentSchema:     json.RawMessage(acceptanceArtifactBinaryUpSchema),
			ResponseMediaTypes: []string{"application/json"},
		},
	}
	artifactBinaryDown := Binding{
		BackendOperationID: "acceptance.artifact.binary.down",
		Effect:             EffectRead,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceArtifactServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "artifactBinaryDown",
		Method:             "GET",
		Path:               "/acceptance/artifact/binary-down",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"application/octet-stream"},
		Generic: &GenericSpec{
			Method:             "GET",
			Path:               "/acceptance/artifact/binary-down",
			ArgumentSchema:     json.RawMessage(acceptanceEmptyArgumentSchema),
			ResponseMediaTypes: []string{"application/octet-stream"},
		},
	}
	artifactMultipartRoundtrip := Binding{
		BackendOperationID: "acceptance.artifact.multipart.roundtrip",
		Effect:             EffectRead,
		Handler:            HandlerGenericSBI,
		ServiceKey:         AcceptanceArtifactServiceKey,
		SourceID:           "acceptance-fixture-openapi",
		SourceOperationID:  "artifactMultipartRoundtrip",
		Method:             "POST",
		Path:               "/acceptance/artifact/multipart-roundtrip",
		RequestMediaType:   "application/json",
		ResponseMediaTypes: []string{"multipart/related"},
		Generic: &GenericSpec{
			Method: "POST",
			Path:   "/acceptance/artifact/multipart-roundtrip",
			Body: &GenericBody{
				Kind:          "multipart",
				MediaType:     "multipart/related",
				MultipartRoot: true,
				MaxParts:      3,
				RequestSchema: json.RawMessage(acceptanceArtifactMultipartBodySchema),
				Parts: []GenericPart{
					{
						Name:             "n1Message",
						ContentType:      "application/vnd.3gpp.5gnas",
						JSONPointerField: "/messages/0/n1MessageContent/contentId",
						Required:         true,
					},
					{
						Name:             "n2Message",
						ContentType:      "application/vnd.3gpp.ngap",
						JSONPointerField: "/messages/1/n2MessageContent/contentId",
						Required:         false,
					},
				},
			},
			ArgumentSchema:     json.RawMessage(acceptanceArtifactMultipartArgumentSchema),
			ResponseMediaTypes: []string{"multipart/related"},
			ResponseParts: []GenericPart{
				{
					Name:             "n2Response",
					ContentType:      "application/vnd.3gpp.ngap",
					JSONPointerField: "/messages/0/n2MessageContent/contentId",
					Required:         true,
				},
				{
					Name:             "n1Response",
					ContentType:      "application/vnd.3gpp.5gnas",
					JSONPointerField: "/messages/1/n1MessageContent/contentId",
					Required:         true,
				},
			},
		},
	}
	return []Binding{echo, mutation, binary, multipart, artifactBinaryUp, artifactBinaryDown, artifactMultipartRoundtrip}
}
