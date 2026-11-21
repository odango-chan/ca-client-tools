// Package providerapp wires the single trusted Operation Catalog to the typed
// OpenAMSM clients and to the generic SBI executor, and runs the resident
// Provider process. It is the only place where deployment environment
// configuration, service availability and handler selection meet. It never
// receives URLs, methods, paths, headers or credentials from a caller.
package providerapp

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"

	"github.com/odango-chan/ca-client-tools/internal/artifactchannel"
	"github.com/odango-chan/ca-client-tools/internal/catalog"
	"github.com/odango-chan/ca-client-tools/internal/openamsm/metrics"
	smfcontext "github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
	"github.com/odango-chan/ca-client-tools/internal/provider"
	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/executor"
	"github.com/odango-chan/ca-client-tools/internal/sbi/payload"
	"github.com/odango-chan/ca-client-tools/internal/schemacheck"
)

// LookupEnv is the trusted environment lookup injected for tests.
type LookupEnv func(string) (string, bool)

type providerRuntime struct {
	catalog   *catalog.Catalog
	services  *trustedServices
	sbiClient *sbi.Client
	mas       *smfcontext.Client
	masInvoke *smfcontext.TypedInvoker
	metrics   *metrics.Client
	executor  *executor.Executor
	bindings  map[string]provider.Binding
	// operations and argumentSchemas are the prebuilt trusted generic
	// invocation contracts. They exist for every cataloged genericSBI binding;
	// availability depends on the trusted service configuration.
	operations      map[string]executor.Operation
	argumentSchemas map[string]*schemacheck.Schema
	// artifactSlots is the derived closed artifactSlots extension per
	// artifact-bearing generic binding. artifact is the configured Host
	// Artifact Store channel client; artifactLimits are its effective bounds.
	artifactSlots  map[string]*provider.ArtifactSlots
	artifact       *artifactchannel.Channel
	artifactLimits provider.ArtifactLimits
}

func newRuntime(lookup LookupEnv, artifactConfig provider.ArtifactChannelConfig) (provider.Runtime, error) {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	cat, err := catalog.Load()
	if err != nil {
		return nil, &provider.CatalogError{Reason: err.Error()}
	}
	if err := installFixtureBindings(cat); err != nil {
		return nil, &provider.CatalogError{Reason: err.Error()}
	}
	if err := catalog.InstallTypedMASBindings(cat); err != nil {
		return nil, &provider.CatalogError{Reason: err.Error()}
	}

	services, err := loadTrustedServices(lookup)
	if err != nil {
		return nil, &provider.ConfigError{Reason: err.Error()}
	}

	result := &providerRuntime{
		catalog:         cat,
		services:        services,
		bindings:        make(map[string]provider.Binding, len(cat.Bindings)),
		operations:      make(map[string]executor.Operation),
		argumentSchemas: make(map[string]*schemacheck.Schema),
		artifactSlots:   make(map[string]*provider.ArtifactSlots),
	}
	if artifactConfig.Configured() {
		// The Provider enforces the Host-advertised bounds independently and
		// never accepts an artifact larger than its own payload/transport
		// bounds. Effective limits are the minimum of both layers.
		payloadLimits, limitErr := payload.ComposeLimits(
			payload.Limits{},
			payload.DefaultMaxTransportBytes,
			payload.DefaultMaxTransportBytes,
		)
		if limitErr != nil {
			return nil, &provider.ConfigError{Reason: "artifact channel payload limits are invalid"}
		}
		effective := provider.ArtifactLimits{
			RequestTimeout:    artifactConfig.RequestTimeout,
			MaxArtifactBytes:  minInt64(artifactConfig.MaxArtifactBytes, payloadLimits.MaxBinaryBytes, payloadLimits.MaxTotalBytes),
			MaxAggregateBytes: minInt64(artifactConfig.MaxAggregateBytes, payloadLimits.MaxTotalBytes),
			MaxAttachments:    minInt(artifactConfig.MaxAttachments, payloadLimits.MaxParts),
		}
		if effective.MaxArtifactBytes < 1 || effective.MaxAggregateBytes < 1 || effective.MaxAttachments < 1 {
			return nil, &provider.ConfigError{Reason: "artifact channel effective limits are invalid"}
		}
		channel, channelErr := artifactchannel.New(artifactchannel.Config{
			BaseURL:           artifactConfig.BaseURL,
			ProcessToken:      artifactConfig.ProcessToken,
			RequestTimeout:    effective.RequestTimeout,
			MaxArtifactBytes:  effective.MaxArtifactBytes,
			MaxAggregateBytes: effective.MaxAggregateBytes,
			MaxAttachments:    effective.MaxAttachments,
		})
		if channelErr != nil {
			return nil, &provider.ConfigError{Reason: "artifact channel configuration is invalid"}
		}
		result.artifact = channel
		result.artifactLimits = effective
	}

	// One shared transport instance serves every trusted service profile: the
	// typed MAS client, the generic SBI executor and the typed metrics client.
	if len(services.sbiConfig.Services) > 0 {
		client, err := sbi.NewClient(services.sbiConfig)
		if err != nil {
			return nil, &provider.ConfigError{Reason: "trusted SBI transport configuration is invalid"}
		}
		result.sbiClient = client
	}
	if services.has(ServiceKeyMASInvoke, transportSBIH2C) && result.sbiClient != nil {
		invoker, err := smfcontext.NewTypedInvokerWithSBI(result.sbiClient, ServiceKeyMASInvoke)
		if err != nil {
			result.Close()
			return nil, &provider.ConfigError{Reason: "MAS Invoke transport configuration is invalid"}
		}
		invoker.SetTimeout(provider.HardMaxCallDuration)
		result.masInvoke = invoker
	}
	if services.has(ServiceKeySMFContext, transportSBIH2C) && result.sbiClient != nil {
		masClient, err := smfcontext.NewClientWithSBI(smfcontext.Config{}, result.sbiClient, ServiceKeySMFContext)
		if err != nil {
			result.Close()
			return nil, &provider.ConfigError{Reason: "SMF context client configuration is invalid"}
		}
		result.mas = masClient
	}
	_, amfOK := services.metricsOrigin(ServiceKeyAMFMetrics)
	_, smfOK := services.metricsOrigin(ServiceKeySMFMetrics)
	if amfOK || smfOK {
		// The typed metrics client keeps its paging/shape/error handling but no
		// longer owns a transport: lifecycle, cancellation, timeout, redirect and
		// size bounds come from the one shared instance.
		amfKey, smfKey := "", ""
		if amfOK {
			amfKey = ServiceKeyAMFMetrics
		}
		if smfOK {
			smfKey = ServiceKeySMFMetrics
		}
		metricsClient, err := metrics.NewClientWithSBI(result.sbiClient, amfKey, smfKey, metrics.Config{})
		if err != nil {
			result.Close()
			return nil, &provider.ConfigError{Reason: "metrics transport profile configuration is invalid"}
		}
		result.metrics = metricsClient
	}
	if result.sbiClient != nil {
		genericExecutor, err := executor.New(result.sbiClient, nil, payload.Limits{})
		if err != nil {
			result.Close()
			return nil, &provider.ConfigError{Reason: "generic SBI executor configuration is invalid"}
		}
		result.executor = genericExecutor
	}

	for _, binding := range cat.Bindings {
		converted, err := convertBinding(binding)
		if err != nil {
			result.Close()
			return nil, &provider.CatalogError{Reason: err.Error()}
		}
		if binding.Handler == catalog.HandlerGenericSBI {
			operation, err := buildGenericOperation(cat, binding)
			if err != nil {
				result.Close()
				return nil, &provider.CatalogError{Reason: err.Error()}
			}
			schema, err := schemacheck.Compile(binding.Generic.ArgumentSchema)
			if err != nil {
				result.Close()
				return nil, &provider.CatalogError{Reason: "generic argument schema is invalid"}
			}
			result.operations[binding.BackendOperationID] = operation
			result.argumentSchemas[binding.BackendOperationID] = schema
			slots := artifactSlotsForOperation(operation)
			result.artifactSlots[binding.BackendOperationID] = slots
			converted.ArtifactSlots = slots
		}
		if binding.Handler == catalog.HandlerMASInvoke {
			if _, known := smfcontext.LookupTypedBinding(binding.SourceOperationID); !known {
				result.Close()
				return nil, &provider.CatalogError{Reason: "typed MAS binding names an unknown callable"}
			}
		}
		converted.Available, converted.UnavailableReason = result.availability(binding)
		result.bindings[binding.BackendOperationID] = converted
	}
	return result, nil
}

func (r *providerRuntime) availability(binding catalog.Binding) (bool, string) {
	switch binding.Handler {
	case catalog.HandlerSMFContextQuery:
		if r.mas == nil {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	case catalog.HandlerAMFMetricsList:
		if _, ok := r.services.metricsOrigin(ServiceKeyAMFMetrics); !ok {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	case catalog.HandlerSMFMetricsList:
		if _, ok := r.services.metricsOrigin(ServiceKeySMFMetrics); !ok {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	case catalog.HandlerMASInvoke:
		if r.masInvoke == nil {
			return false, "NOT_CONFIGURED"
		}
		return true, ""
	case catalog.HandlerGenericSBI:
		if r.executor == nil || !r.services.has(binding.ServiceKey, transportSBIH2C) {
			return false, "NOT_CONFIGURED"
		}
		operation, ok := r.operations[binding.BackendOperationID]
		if !ok {
			return false, "NOT_IMPLEMENTED"
		}
		slots := r.artifactSlots[binding.BackendOperationID]
		if slots.Configured() {
			// P1: artifact-bearing bindings are available exactly when the Host
			// configured an artifact channel for this process.
			if r.artifact == nil {
				return false, "NOT_CONFIGURED"
			}
			return true, ""
		}
		// A non-artifact generic response must still be a projectable JSON or
		// empty body.
		if !responseProjectionSupported(operation) {
			return false, "NOT_IMPLEMENTED"
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
	converted := provider.Binding{
		BackendOperationID: binding.BackendOperationID,
		Effect:             string(binding.Effect),
		HandlerProfile:     binding.Handler,
		SourceID:           binding.SourceID,
		SourceOperationID:  binding.SourceOperationID,
		RequestMediaType:   binding.RequestMediaType,
		ResponseMediaTypes: binding.ResponseMediaTypes,
		Arguments:          arguments,
	}
	if binding.Generic != nil {
		converted.ArgumentSchema = binding.Generic.ArgumentSchema
	}
	return converted, nil
}

func (r *providerRuntime) CatalogVersion() string { return r.catalog.Version }
func (r *providerRuntime) CatalogDigest() string  { return r.catalog.Digest }

// ArtifactLimits reports the effective Host channel bounds when configured.
func (r *providerRuntime) ArtifactLimits() (provider.ArtifactLimits, bool) {
	if r.artifact == nil {
		return provider.ArtifactLimits{}, false
	}
	return r.artifactLimits, true
}

func (r *providerRuntime) BindingIDs() []string { return r.catalog.IDs() }

func (r *providerRuntime) Binding(id string) (provider.Binding, bool) {
	binding, ok := r.bindings[id]
	return binding, ok
}

func (r *providerRuntime) Invoke(ctx context.Context, backendOperationID, invocationID string, arguments map[string]any) (provider.Result, error) {
	return r.invoke(ctx, backendOperationID, invocationID, arguments, nil)
}

// InvokeWithHostInvocation consumes the Host-private occurrence scope. Only the
// typed MAS handler may see it; every other binding rejects it before I/O.
func (r *providerRuntime) InvokeWithHostInvocation(ctx context.Context, backendOperationID, invocationID string,
	arguments map[string]any, host provider.HostInvocation) (provider.Result, error) {
	return r.invoke(ctx, backendOperationID, invocationID, arguments, &host)
}

func (r *providerRuntime) invoke(ctx context.Context, backendOperationID, invocationID string,
	arguments map[string]any, host *provider.HostInvocation) (provider.Result, error) {
	binding, ok := r.bindings[backendOperationID]
	if !ok || !binding.Available {
		reason := "NOT_CONFIGURED"
		if binding.UnavailableReason != "" {
			reason = binding.UnavailableReason
		}
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": reason},
		}
	}
	// The Host-private scope belongs to exactly one handler. A caller that
	// attaches it to another binding is rejected before any remote I/O, even
	// when the binding is otherwise available.
	if host != nil && binding.HandlerProfile != catalog.HandlerMASInvoke {
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	// Production Mutation remains gated: reject before any remote I/O even when
	// the binding is otherwise available. The typed MAS handler is the one
	// authorized write path: it is available only when the deployment
	// configured the private orchestration endpoint and the Host supplied the
	// exact occurrence scope.
	if binding.Effect == string(catalog.EffectMutation) && binding.HandlerProfile != catalog.HandlerMASInvoke {
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": "MUTATION_GATED"},
		}
	}
	catalogBinding, ok := r.catalog.Find(backendOperationID)
	if !ok {
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendOperationUnsupported}
	}
	switch catalogBinding.Handler {
	case catalog.HandlerSMFContextQuery:
		return r.invokeSMFContext(ctx, catalogBinding, arguments)
	case catalog.HandlerAMFMetricsList:
		return r.invokeAMFMetrics(ctx, catalogBinding.BackendOperationID, arguments)
	case catalog.HandlerSMFMetricsList:
		return r.invokeSMFMetrics(ctx, arguments)
	case catalog.HandlerMASInvoke:
		return r.invokeTypedMAS(ctx, catalogBinding, arguments, host)
	case catalog.HandlerGenericSBI:
		return r.invokeGeneric(ctx, catalogBinding.BackendOperationID, invocationID, arguments)
	default:
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": "HANDLER_NOT_WIRED"},
		}
	}
}

// invokeTypedMAS submits one fixed callable to the private OpenAMSM MAS
// orchestration endpoint. The returned Provider result is an observation only:
// it carries the Owner response code, HTTP status, projected outputs and the
// emitted smContextRef. It is not a commit receipt and grants no Outcome.
func (r *providerRuntime) invokeTypedMAS(ctx context.Context, binding catalog.Binding,
	arguments map[string]any, host *provider.HostInvocation) (provider.Result, error) {
	if r.masInvoke == nil {
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": "NOT_CONFIGURED"},
		}
	}
	if host == nil {
		// Never synthesize a Host occurrence identity from caller arguments.
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	scope := smfcontext.HostInvocation{
		Version:              host.Version,
		DeploymentID:         host.DeploymentID,
		OwnerInstanceID:      host.OwnerInstanceID,
		OriginalInvocationID: host.OriginalInvocationID,
		SessionRef:           host.SessionRef,
		SMContextRef:         host.SMContextRef,
		SUPI:                 host.SUPI,
		PSI:                  int(host.PSI),
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	observation, err := r.masInvoke.Invoke(ctx, scope, binding.SourceOperationID, encoded)
	if err != nil {
		var inputErr *smfcontext.InputError
		if errors.As(err, &inputErr) {
			return provider.Result{}, &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
		}
		if ctx != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
			return provider.Result{}, ctx.Err()
		}
		return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteTransportError}
	}
	outputs := make(map[string]any, len(observation.Outputs))
	outputNames := append([]string(nil), observation.RawOutputNames...)
	if len(outputNames) == 0 {
		for name, value := range observation.Outputs {
			outputs[name] = value
			outputNames = append(outputNames, name)
		}
	} else {
		for name, value := range observation.Outputs {
			outputs[name] = value
		}
	}
	sort.Strings(outputNames)
	response := map[string]any{
		"observation": map[string]any{
			"code":        observation.Code,
			"httpStatus":  observation.HTTPStatus,
			"outputs":     outputs,
			"outputNames": outputNames,
		},
	}
	if observation.SMContextRef != "" {
		response["smContextRef"] = observation.SMContextRef
	}
	return provider.Result{HTTPStatus: observation.HTTPStatus, Response: response}, nil
}

func (r *providerRuntime) invokeGeneric(ctx context.Context, backendOperationID, invocationID string, arguments map[string]any) (provider.Result, error) {
	operation, ok := r.operations[backendOperationID]
	if !ok {
		return provider.Result{}, &provider.Fault{Code: provider.CodeBackendOperationUnsupported}
	}
	slots := r.artifactSlots[backendOperationID]
	if slots.Configured() {
		// Artifact-bearing operations are available only with a configured Host
		// channel; reject at the execution entrance before any remote I/O even
		// if availability reporting is bypassed.
		if r.artifact == nil {
			return provider.Result{}, &provider.Fault{
				Code:   provider.CodeBackendOperationUnsupported,
				Detail: map[string]any{"reason": "NOT_CONFIGURED"},
			}
		}
	} else if !responseProjectionSupported(operation) {
		return provider.Result{}, &provider.Fault{
			Code:   provider.CodeBackendOperationUnsupported,
			Detail: map[string]any{"reason": "NOT_IMPLEMENTED"},
		}
	}
	// Recursive validation of the closed nested invocation schema happens
	// before the executor opens any transport connection.
	if schema := r.argumentSchemas[backendOperationID]; schema != nil {
		if err := schema.Validate(arguments); err != nil {
			return provider.Result{}, &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
		}
	}
	// Contract §2.4/§5.3: a reference value is valid only at a declared artifact
	// slot of the invocation. The declared set is empty for a binding that
	// declares no artifact slots, so a reference smuggled through an open nested
	// argument object is rejected before any remote I/O (and before any channel
	// request) instead of being forwarded to the upstream NF.
	if argumentFault := validateArtifactArguments(slots, arguments); argumentFault != nil {
		return provider.Result{}, argumentFault
	}
	execution := executor.Execution{}
	if slots.Configured() {
		quota := artifactchannel.NewQuota(r.artifactLimits.MaxAggregateBytes, r.artifactLimits.MaxAttachments)
		inputs, resolveErr := r.resolveArtifactInputs(ctx, invocationID, slots, arguments, quota)
		if resolveErr != nil {
			return provider.Result{}, mapArtifactInvocError(ctx, resolveErr)
		}
		sink := r.artifact.NewSink(invocationID, quota)
		execution = executor.Execution{
			Resolver: &artifactInputResolver{inputs: inputs},
			Sink:     sink,
		}
		executed, err := r.executor.ExecuteWith(ctx, operation, executorArgumentsForArtifacts(slots, arguments), execution)
		if err != nil {
			var remoteErr *executor.RemoteError
			if errors.As(err, &remoteErr) {
				problem := &provider.ProblemDetails{Status: remoteErr.HTTPStatus}
				if remoteErr.Problem != nil {
					problem.Status = remoteErr.Problem.Status
					problem.Cause = remoteErr.Problem.Cause
				}
				if problem.Cause == "" {
					problem.Cause = remoteErr.Cause
				}
				return provider.Result{HTTPStatus: remoteErr.HTTPStatus, Problem: problem}, nil
			}
			if mapped := mapArtifactInvocError(ctx, err); mapped != nil {
				return provider.Result{}, mapped
			}
			return provider.Result{}, mapExecutorError(ctx, err)
		}
		if len(slots.Outputs) == 0 {
			// Input-only artifact binding with a JSON response contract.
			return projectGenericResult(executed)
		}
		return projectArtifactResult(executed, slots, sink)
	}
	executed, err := r.executor.Execute(ctx, operation, arguments)
	if err != nil {
		// A validated remote ProblemDetails response is a successful Provider
		// protocol result, not a protocol failure.
		var remoteErr *executor.RemoteError
		if errors.As(err, &remoteErr) {
			problem := &provider.ProblemDetails{Status: remoteErr.HTTPStatus}
			if remoteErr.Problem != nil {
				problem.Status = remoteErr.Problem.Status
				problem.Cause = remoteErr.Problem.Cause
			}
			if problem.Cause == "" {
				problem.Cause = remoteErr.Cause
			}
			return provider.Result{HTTPStatus: remoteErr.HTTPStatus, Problem: problem}, nil
		}
		return provider.Result{}, mapExecutorError(ctx, err)
	}
	return projectGenericResult(executed)
}

// mapArtifactInvocError maps a Host channel failure to the two contract
// Provider faults, preserving caller cancellation/deadline as context errors.
func mapArtifactInvocError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	if code, ok := artifactchannel.FaultCode(err); ok {
		return &provider.Fault{Code: code}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// projectGenericResult projects a successfully executed generic SBI response.
// Binary and multipart/related projection is P1 (Host Artifact Store): any
// body bytes or attachments are rejected before the JSON root branch, so a
// multipart response that carries both a JSON root and attachments can never be
// returned as a partial success with the attachments silently discarded. The
// fault carries no attachment content or metadata.
func projectGenericResult(executed executor.Result) (provider.Result, error) {
	switch {
	case len(executed.Body) > 0 || len(executed.Attachments) > 0:
		return provider.Result{}, &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
	case executed.JSON != nil:
		return provider.Result{HTTPStatus: executed.HTTPStatus, Response: executed.JSON}, nil
	default:
		return provider.Result{HTTPStatus: executed.HTTPStatus}, nil
	}
}

func mapExecutorError(ctx context.Context, err error) error {
	if ctx != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return ctx.Err()
	}
	var inputErr *executor.InputError
	if errors.As(err, &inputErr) {
		return &provider.Fault{Code: provider.CodeBackendArgumentsInvalid}
	}
	var configErr *executor.ConfigurationError
	if errors.As(err, &configErr) {
		return &provider.Fault{Code: provider.CodeProviderConfigurationInvalid}
	}
	var protocolErr *executor.ProtocolError
	if errors.As(err, &protocolErr) {
		return &provider.Fault{Code: provider.CodeRemoteResponseInvalid}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &provider.Fault{Code: provider.CodeCallDeadlineExceeded}
	}
	if errors.Is(err, context.Canceled) {
		return &provider.Fault{Code: provider.CodeCallCancelled}
	}
	return &provider.Fault{Code: provider.CodeRemoteTransportError}
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
	// One transport instance owns every profile's connections; closing it also
	// releases the connections used by typed metrics and MAS dispatch.
	if r.sbiClient != nil {
		r.sbiClient.CloseIdleConnections()
	}
	if r.artifact != nil {
		r.artifact.CloseIdleConnections()
	}
}

func minInt64(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
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
func EnvSummary(lookup LookupEnv) map[string]any {
	summary := map[string]any{}
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
	if _, ok := lookup(EnvMASInvokeOrigin); ok {
		summary["masInvokeConfigured"] = true
	}
	if _, ok := lookup(EnvServiceConfig); ok {
		summary["serviceConfigConfigured"] = true
	}
	return summary
}
