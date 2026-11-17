// Package providerapp wires the trusted Operation Catalog to the typed
// OpenAMSM clients and runs the resident Provider process. It is the only place
// where deployment environment configuration, service availability and typed
// handler selection meet. It never receives URLs, methods, paths, headers or
// credentials from a caller.
package providerapp

import (
	"context"
	"errors"
	"regexp"

	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/openamsm/metrics"
	smfcontext "github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
	"github.com/odango-chan/ca-client-tools/internal/provider"
)

// Trusted deployment environment variables for the resident Provider. These
// are the only configuration inputs; they never appear in protocol frames.
const (
	EnvSMFSBIOrigin     = "CA_OPENAMSM_SMF_SBI_ORIGIN"
	EnvQueryToken       = "CA_OPENAMSM_QUERY_TOKEN"
	EnvAMFMetricsOrigin = "CA_OPENAMSM_AMF_METRICS_ORIGIN"
	EnvSMFMetricsOrigin = "CA_OPENAMSM_SMF_METRICS_ORIGIN"
)

// LookupEnv is the trusted environment lookup injected for tests.
type LookupEnv func(string) (string, bool)

type providerRuntime struct {
	version  string
	catalog  *catalog.Catalog
	mas      *smfcontext.Client
	metrics  *metrics.Client
	amfReady bool
	smfReady bool
	bindings map[string]provider.Binding
}

func newRuntime(protocolVersion string, lookup LookupEnv) (provider.Runtime, error) {
	switch protocolVersion {
	case provider.ProtocolV1:
		return newV1Runtime(lookup)
	case provider.ProtocolV2:
		return newV2Runtime(lookup)
	default:
		return nil, &provider.ConfigError{Reason: "unsupported protocol version"}
	}
}

func newV1Runtime(lookup LookupEnv) (provider.Runtime, error) {
	origin, originOK := lookup(EnvSMFSBIOrigin)
	token, tokenOK := lookup(EnvQueryToken)
	if !originOK || !tokenOK {
		return nil, &provider.ConfigError{Reason: "SMF SBI origin or query token is not configured"}
	}
	masClient, err := smfcontext.NewClient(smfcontext.Config{Origin: origin, QueryToken: token})
	if err != nil {
		return nil, &provider.ConfigError{Reason: "SMF SBI client configuration is invalid"}
	}
	cat, err := catalog.LoadV1()
	if err != nil {
		return nil, &provider.CatalogError{Reason: err.Error()}
	}
	result := &providerRuntime{
		version:  cat.Version,
		catalog:  cat,
		mas:      masClient,
		bindings: make(map[string]provider.Binding, len(cat.Bindings)),
	}
	for _, binding := range cat.Bindings {
		// The frozen v1 catalog intentionally carries no typed argument schema:
		// Protocol v1 fixes the arguments to exactly supi + pduSessionId. The v1
		// runtime therefore attaches that exact private argument profile so the
		// same provider-layer validation applies to both protocol versions.
		binding.Arguments = v1SMFContextArguments()
		converted, err := convertBinding(binding)
		if err != nil {
			masClient.CloseIdleConnections()
			return nil, &provider.CatalogError{Reason: err.Error()}
		}
		converted.Available = true
		result.bindings[binding.BackendOperationID] = converted
	}
	return result, nil
}

// v1SMFContextArguments is the frozen Protocol v1 invoke argument profile.
func v1SMFContextArguments() []catalog.Argument {
	minimum := int64(0)
	maximum := int64(255)
	return []catalog.Argument{
		{Name: "supi", Type: "string", Required: true, Pattern: "^imsi-[0-9]{1,15}$"},
		{Name: "pduSessionId", Type: "integer", Required: true, Minimum: &minimum, Maximum: &maximum},
	}
}

func newV2Runtime(lookup LookupEnv) (provider.Runtime, error) {
	cat, err := catalog.LoadV2()
	if err != nil {
		return nil, &provider.CatalogError{Reason: err.Error()}
	}

	sbiOrigin, sbiOriginOK := lookup(EnvSMFSBIOrigin)
	queryToken, tokenOK := lookup(EnvQueryToken)
	if sbiOriginOK != tokenOK {
		return nil, &provider.ConfigError{Reason: "SMF SBI origin and query token must be configured together"}
	}
	amfOrigin, amfOK := lookup(EnvAMFMetricsOrigin)
	smfMetricsOrigin, smfOK := lookup(EnvSMFMetricsOrigin)
	if !sbiOriginOK && !amfOK && !smfOK {
		return nil, &provider.ConfigError{Reason: "no trusted OpenAMSM service origin is configured"}
	}

	var masClient *smfcontext.Client
	if sbiOriginOK {
		masClient, err = smfcontext.NewClient(smfcontext.Config{Origin: sbiOrigin, QueryToken: queryToken})
		if err != nil {
			return nil, &provider.ConfigError{Reason: "SMF SBI client configuration is invalid"}
		}
	}

	var metricsClient *metrics.Client
	if amfOK || smfOK {
		metricsConfig := metrics.Config{}
		if amfOK {
			metricsConfig.AMFOrigin = amfOrigin
		}
		if smfOK {
			metricsConfig.SMFOrigin = smfMetricsOrigin
		}
		metricsClient, err = metrics.NewClient(metricsConfig)
		if err != nil {
			if masClient != nil {
				masClient.CloseIdleConnections()
			}
			return nil, &provider.ConfigError{Reason: "metrics client configuration is invalid"}
		}
	}

	result := &providerRuntime{
		version:  cat.Version,
		catalog:  cat,
		mas:      masClient,
		metrics:  metricsClient,
		amfReady: amfOK,
		smfReady: smfOK,
		bindings: make(map[string]provider.Binding, len(cat.Bindings)),
	}
	for _, binding := range cat.Bindings {
		converted, err := convertBinding(binding)
		if err != nil {
			result.Close()
			return nil, &provider.CatalogError{Reason: err.Error()}
		}
		converted.Available, converted.UnavailableReason = availability(binding, masClient != nil, amfOK, smfOK)
		result.bindings[binding.BackendOperationID] = converted
	}
	return result, nil
}

func availability(binding catalog.Binding, masConfigured, amfConfigured, smfMetricsConfigured bool) (bool, string) {
	switch binding.Handler {
	case catalog.HandlerSMFContextQuery:
		if !masConfigured {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	case catalog.HandlerAMFMetricsList:
		if !amfConfigured {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	case catalog.HandlerSMFMetricsList:
		if !smfMetricsConfigured {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	default:
		return false, "NOT_CONFIGURED"
	}
}

func convertBinding(binding catalog.Binding) (provider.Binding, error) {
	arguments := make([]provider.Argument, 0, len(binding.Arguments))
	for _, argument := range binding.Arguments {
		converted := provider.Argument{
			Name:     argument.Name,
			Type:     argument.Type,
			Required: argument.Required,
			Minimum:  argument.Minimum,
			Maximum:  argument.Maximum,
			Default:  argument.Default,
		}
		if argument.Pattern != "" {
			pattern, err := regexp.Compile(argument.Pattern)
			if err != nil {
				return provider.Binding{}, errors.New("invalid catalog argument pattern")
			}
			converted.Pattern = pattern
		}
		arguments = append(arguments, converted)
	}
	return provider.Binding{
		BackendOperationID: binding.BackendOperationID,
		Effect:             string(binding.Effect),
		HandlerProfile:     binding.Handler,
		SourceID:           binding.SourceID,
		SourceOperationID:  binding.SourceOperationID,
		RequestMediaType:   binding.RequestMediaType,
		ResponseMediaTypes: binding.ResponseMediaTypes,
		Arguments:          arguments,
	}, nil
}

func (r *providerRuntime) CatalogVersion() string { return r.version }
func (r *providerRuntime) CatalogDigest() string  { return r.catalog.Digest }

func (r *providerRuntime) BindingIDs() []string { return r.catalog.IDs() }

func (r *providerRuntime) Binding(id string) (provider.Binding, bool) {
	binding, ok := r.bindings[id]
	return binding, ok
}

func (r *providerRuntime) Invoke(ctx context.Context, backendOperationID string, arguments map[string]any) (provider.Result, error) {
	if binding, ok := r.bindings[backendOperationID]; !ok || !binding.Available {
		reason := "NOT_CONFIGURED"
		if binding.UnavailableReason != "" {
			reason = binding.UnavailableReason
		}
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": reason},
		}
	}
	binding, ok := r.catalog.Find(backendOperationID)
	if !ok {
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendOperationUnsupported}
	}
	switch binding.Handler {
	case catalog.HandlerSMFContextQuery:
		return r.invokeSMFContext(ctx, binding, arguments)
	case catalog.HandlerAMFMetricsList:
		return r.invokeAMFMetrics(ctx, binding.BackendOperationID, arguments)
	case catalog.HandlerSMFMetricsList:
		return r.invokeSMFMetrics(ctx, arguments)
	default:
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": "HANDLER_NOT_WIRED"},
		}
	}
}

func (r *providerRuntime) invokeSMFContext(ctx context.Context, binding catalog.Binding, arguments map[string]any) (provider.Result, error) {
	supi, _ := arguments["supi"].(string)
	psiValue, _ := arguments["pduSessionId"].(int64)
	psi := int(psiValue)
	outcome, err := r.mas.QueryOutcome(ctx, smfcontext.Query{
		MAS:          binding.ContextType,
		SUPI:         supi,
		PDUSessionID: &psi,
	})
	if err != nil {
		return provider.Result{}, mapSMFContextError(err)
	}
	result := provider.Result{HTTPStatus: outcome.HTTPStatus}
	switch {
	case outcome.Result != nil:
		result.Response = outcome.Result
	case outcome.Problem != nil:
		result.Problem = &provider.ProblemDetails{Status: outcome.Problem.Status, Cause: outcome.Problem.Cause}
	default:
		return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
	}
	return result, nil
}

func (r *providerRuntime) invokeAMFMetrics(ctx context.Context, backendOperationID string, arguments map[string]any) (provider.Result, error) {
	page, pageSize := pageArguments(arguments)
	var (
		result metrics.Page
		err    error
	)
	switch backendOperationID {
	case "openamsm.amf.ue-info.list":
		result, err = r.metrics.GetUEInfo(ctx, page, pageSize)
	case "openamsm.amf.gnb-info.list":
		result, err = r.metrics.GetGNBInfo(ctx, page, pageSize)
	default:
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendOperationUnsupported}
	}
	if err != nil {
		return provider.Result{}, mapMetricsError(err)
	}
	return provider.Result{HTTPStatus: 200, Response: result}, nil
}

func (r *providerRuntime) invokeSMFMetrics(ctx context.Context, arguments map[string]any) (provider.Result, error) {
	page, pageSize := pageArguments(arguments)
	result, err := r.metrics.GetPDUInfo(ctx, page, pageSize)
	if err != nil {
		return provider.Result{}, mapMetricsError(err)
	}
	return provider.Result{HTTPStatus: 200, Response: result}, nil
}

func pageArguments(arguments map[string]any) (int, int) {
	page := 0
	pageSize := 100
	if value, ok := arguments["page"].(int64); ok {
		page = int(value)
	}
	if value, ok := arguments["pageSize"].(int64); ok {
		pageSize = int(value)
	}
	return page, pageSize
}

func (r *providerRuntime) Close() {
	if r.mas != nil {
		r.mas.CloseIdleConnections()
	}
	if r.metrics != nil {
		r.metrics.CloseIdleConnections()
	}
}

func mapSMFContextError(err error) error {
	var inputErr *smfcontext.InputError
	if errors.As(err, &inputErr) {
		return &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	var configErr *smfcontext.ConfigurationError
	if errors.As(err, &configErr) {
		return &provider.Fault{Code: provider.CodeProviderConfigurationInvalid}
	}
	var protocolErr *smfcontext.ProtocolError
	if errors.As(err, &protocolErr) {
		return &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
	}
	var remoteErr *smfcontext.RemoteError
	if errors.As(err, &remoteErr) {
		return &provider.Fault{Code: provider.CodeRemoteResponseInvalid, Detail: map[string]any{"httpStatus": remoteErr.HTTPStatus}}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &provider.Fault{Code: provider.CodeRemoteTransportError}
	}
	return &provider.Fault{Code: provider.CodeRemoteTransportError}
}

func mapMetricsError(err error) error {
	var inputErr *metrics.InputError
	if errors.As(err, &inputErr) {
		return &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	var configErr *metrics.ConfigurationError
	if errors.As(err, &configErr) {
		return &provider.Fault{Code: provider.CodeProviderConfigurationInvalid}
	}
	var remoteErr *metrics.RemoteError
	if errors.As(err, &remoteErr) {
		return &provider.Fault{Code: provider.CodeRemoteResponseInvalid, Detail: map[string]any{"httpStatus": remoteErr.HTTPStatus}}
	}
	var protocolErr *metrics.ProtocolError
	if errors.As(err, &protocolErr) {
		return &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
	}
	return &provider.Fault{Code: provider.CodeRemoteTransportError}
}

// EnvSummary is a safe diagnostic summary of the trusted configuration. It
// never contains origins, tokens or subscriber data.
func EnvSummary(protocolVersion string, lookup LookupEnv) map[string]any {
	summary := map[string]any{"protocolVersion": protocolVersion}
	if _, ok := lookup(EnvSMFSBIOrigin); ok {
		summary["smfSbiConfigured"] = true
	}
	if _, ok := lookup(EnvQueryToken); ok {
		summary["queryTokenConfigured"] = true
	}
	if _, ok := lookup(EnvAMFMetricsOrigin); ok {
		summary["amfMetricsConfigured"] = true
	}
	if _, ok := lookup(EnvSMFMetricsOrigin); ok {
		summary["smfMetricsConfigured"] = true
	}
	return summary
}
