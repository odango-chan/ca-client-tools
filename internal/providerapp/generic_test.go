package providerapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
)

func genericCatalogBinding() catalog.Binding {
	pageDefault := int64(2)
	return catalog.Binding{
		BackendOperationID: "openamsm.test.generic.read",
		Effect:             catalog.EffectRead,
		Handler:            catalog.HandlerGenericSBI,
		ServiceKey:         "test-service",
		ResponseMediaTypes: []string{"application/json"},
		Generic: &catalog.GenericSpec{
			Method: "POST",
			Path:   "/test/{id}",
			PathParameters: []catalog.Argument{
				{Name: "id", Type: "string", Required: true},
			},
			QueryParameters: []catalog.Argument{
				{Name: "filter", Type: "string", Required: true},
				{Name: "page", Type: "integer", Required: false, Default: pageDefault},
			},
			HeaderBindings: []catalog.HeaderBinding{{Name: "X-Trusted", Value: "yes"}},
			Body: &catalog.GenericBody{
				Kind:      "json",
				MediaType: "application/json",
				RequestSchema: json.RawMessage(`{
					"type": "object",
					"additionalProperties": false,
					"required": ["name"],
					"properties": {"name": {"type": "string"}}
				}`),
			},
			ResponseSchema: json.RawMessage(`{
				"type": "object",
				"additionalProperties": false,
				"required": ["ok"],
				"properties": {"ok": {"type": "boolean"}}
			}`),
		},
	}
}

func newGenericTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server.Config.Protocols = protocols
	server.Start()
	t.Cleanup(server.Close)
	return server
}

// TestGenericBridgeCatalogToExecutor proves the trusted-catalog generic
// binding is executed by the generic SBI executor without any new typed client:
// method, path template, typed query/path parameters, trusted header, JSON body
// schema and response schema all come from the catalog binding.
func TestGenericBridgeCatalogToExecutor(t *testing.T) {
	type observed struct {
		method      string
		path        string
		query       string
		trusted     string
		body        map[string]any
		contentType string
	}
	observations := make(chan observed, 1)
	server := newGenericTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		observations <- observed{
			method:      request.Method,
			path:        request.URL.Path,
			query:       request.URL.RawQuery,
			trusted:     request.Header.Get("X-Trusted"),
			body:        decoded,
			contentType: request.Header.Get("Content-Type"),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"ok":true}`)
	}))

	client, err := sbi.NewClient(sbi.Config{
		Services: map[string]sbi.ServiceConfig{
			"test-service": {Origin: server.URL},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)

	catalogValue := &catalog.Catalog{
		APIVersion: catalog.APIVersionV2,
		Version:    "2",
		Schemas:    map[string]json.RawMessage{},
	}
	operation, err := buildGenericOperation(catalogValue, genericCatalogBinding())
	if err != nil {
		t.Fatalf("buildGenericOperation: %v", err)
	}
	genericExecutor, err := executor.New(client, nil, payload.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := genericExecutor.Execute(context.Background(), operation, map[string]any{
		"id":     "abc",
		"filter": "sessions",
		"body":   map[string]any{"name": "demo"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.HTTPStatus != http.StatusOK {
		t.Fatalf("httpStatus = %d", result.HTTPStatus)
	}
	response, ok := result.JSON.(map[string]any)
	if !ok || response["ok"] != true {
		t.Fatalf("unexpected decoded response: %#v", result.JSON)
	}
	got := <-observations
	if got.method != http.MethodPost || got.path != "/test/abc" || got.query != "filter=sessions&page=2" {
		t.Fatalf("unexpected request: %#v", got)
	}
	if got.trusted != "yes" || got.contentType != "application/json" {
		t.Fatalf("trusted header/content type not applied: %#v", got)
	}
	if got.body["name"] != "demo" {
		t.Fatalf("body = %#v", got.body)
	}
}

func TestGenericBridgeMapsMultipartParts(t *testing.T) {
	binding := genericCatalogBinding()
	binding.Generic.Body = &catalog.GenericBody{
		Kind:             "multipart",
		MediaType:        "multipart/related",
		RequestSchema:    json.RawMessage(`{"type":"object"}`),
		MultipartRoot:    true,
		MaxParts:         2,
		ContentTypeParam: "3gpp",
		Parts: []catalog.GenericPart{{
			Name:             "n1Message",
			ContentType:      "application/vnd.3gpp.5gnas",
			JSONPointerField: "/n1MessageContent",
			Required:         true,
		}},
	}
	binding.Generic.ResponseMediaTypes = []string{"multipart/related"}
	binding.Generic.ResponseSchema = json.RawMessage(`{"type":"object"}`)
	binding.Generic.ResponseParts = []catalog.GenericPart{{
		Name:             "n2Message",
		ContentType:      "application/vnd.3gpp.ngap",
		JSONPointerField: "/n2MessageContent",
		Required:         true,
	}}

	operation, err := buildGenericOperation(&catalog.Catalog{}, binding)
	if err != nil {
		t.Fatalf("buildGenericOperation: %v", err)
	}
	if operation.Body == nil || len(operation.Body.Parts) != 1 {
		t.Fatalf("request multipart parts = %#v, want one declared part", operation.Body)
	}
	requestPart := operation.Body.Parts[0]
	if requestPart.Name != "n1Message" || requestPart.ContentType != "application/vnd.3gpp.5gnas" ||
		requestPart.JSONPointerField != "/n1MessageContent" || !requestPart.Required {
		t.Fatalf("request multipart part mapping = %+v", requestPart)
	}
	if len(operation.ResponseParts) != 1 {
		t.Fatalf("response multipart parts = %+v, want one declared part", operation.ResponseParts)
	}
	responsePart := operation.ResponseParts[0]
	if responsePart.Name != "n2Message" || responsePart.ContentType != "application/vnd.3gpp.ngap" ||
		responsePart.JSONPointerField != "/n2MessageContent" || !responsePart.Required {
		t.Fatalf("response multipart part mapping = %+v", responsePart)
	}
	if len(operation.QueryParameters) != 2 || operation.QueryParameters[1].Default != int64(2) {
		t.Fatalf("query parameter defaults were not mapped: %+v", operation.QueryParameters)
	}
}

func TestGenericBridgeRejectsInvalidArgumentsBeforeNetwork(t *testing.T) {
	var requests int
	server := newGenericTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"ok":true}`)
	}))
	client, err := sbi.NewClient(sbi.Config{Services: map[string]sbi.ServiceConfig{
		"test-service": {Origin: server.URL},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	operation, err := buildGenericOperation(&catalog.Catalog{}, genericCatalogBinding())
	if err != nil {
		t.Fatal(err)
	}
	genericExecutor, err := executor.New(client, nil, payload.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string]map[string]any{
		"missing body field":    {"id": "abc", "filter": "x", "body": map[string]any{}},
		"path value with slash": {"id": "a/b", "filter": "x", "body": map[string]any{"name": "n"}},
		"unknown argument":      {"id": "abc", "filter": "x", "body": map[string]any{"name": "n"}, "url": "http://evil"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := genericExecutor.Execute(context.Background(), operation, args); err == nil {
				t.Fatal("expected input error")
			}
		})
	}
	if requests != 0 {
		t.Fatalf("invalid arguments reached the network %d times", requests)
	}
}

func TestGenericBridgeMutationRequiresIdempotency(t *testing.T) {
	binding := genericCatalogBinding()
	binding.Effect = catalog.EffectMutation
	operation, err := buildGenericOperation(&catalog.Catalog{}, binding)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Idempotency != nil {
		t.Fatal("idempotency unexpectedly non-nil")
	}
	server := newGenericTestServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("mutation without idempotency reached the network")
	}))
	client, err := sbi.NewClient(sbi.Config{Services: map[string]sbi.ServiceConfig{
		"test-service": {Origin: server.URL},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	genericExecutor, err := executor.New(client, nil, payload.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, executeErr := genericExecutor.Execute(context.Background(), operation, map[string]any{
		"id": "abc", "filter": "x", "body": map[string]any{"name": "n"},
	})
	if executeErr == nil {
		t.Fatal("mutation without an idempotency contract must be rejected")
	}
	var configErr *executor.ConfigurationError
	if !errors.As(executeErr, &configErr) {
		t.Fatalf("error = %v, want ConfigurationError", executeErr)
	}
}
