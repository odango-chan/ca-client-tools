package providerapp

import (
	"fmt"

	"github.com/odango-chan/ca-client-tools/internal/openamsm/metrics"
	"github.com/odango-chan/ca-client-tools/internal/sbi"
	"github.com/odango-chan/ca-client-tools/internal/sbi/serviceconfig"
)

// Trusted deployment configuration inputs for the resident Provider. Origins,
// tokens and the optional service-configuration file are deployment content;
// they never appear in protocol frames or diagnostics.
const (
	EnvSMFSBIOrigin     = "CA_OPENAMSM_SMF_SBI_ORIGIN"
	EnvQueryToken       = "CA_OPENAMSM_QUERY_TOKEN"
	EnvAMFMetricsOrigin = "CA_OPENAMSM_AMF_METRICS_ORIGIN"
	EnvSMFMetricsOrigin = "CA_OPENAMSM_SMF_METRICS_ORIGIN"
	// EnvServiceConfig optionally points at the trusted multi-service SBI
	// configuration file. It supplies additional h2c services (for example a
	// cataloged genericSBI operation) and their token profiles.
	EnvServiceConfig = "CA_PROVIDER_SERVICE_CONFIG"

	// Service keys are the Catalog serviceKey values.
	ServiceKeySMFContext = "openamsm-smf-context"
	ServiceKeyAMFMetrics = metrics.AMFMetricsServiceKey
	ServiceKeySMFMetrics = metrics.SMFMetricsServiceKey

	tokenProfileOpenAMSMQuery = "openamsm-query-token"
	queryTokenHeader          = "X-OpenAMSM-Query-Token"
)

// transportProfile records the reviewed transport for one trusted service.
// Standard SBI services use cleartext HTTP/2 prior knowledge; the OpenAMSM
// metrics listeners keep their source-verified plain HTTP/1.1 typed client.
type transportProfile string

const (
	transportSBIH2C       transportProfile = "sbi-h2c"
	transportMetricsHTTP1 transportProfile = "metrics-http1"
)

// trustedService is one validated trusted endpoint profile addressed by the
// Catalog serviceKey.
type trustedService struct {
	transport          transportProfile
	origin             string
	apiRoot            string
	headerTokenProfile string
}

// trustedServices is the single trusted service registry resolved at Provider
// startup. Typed and generic dispatch both consult it; callers never can.
type trustedServices struct {
	byKey     map[string]trustedService
	sbiConfig sbi.Config
}

func (s *trustedServices) has(key string, profile transportProfile) bool {
	if s == nil {
		return false
	}
	service, ok := s.byKey[key]
	return ok && service.transport == profile
}

func (s *trustedServices) metricsOrigin(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	service, ok := s.byKey[key]
	if !ok || service.transport != transportMetricsHTTP1 {
		return "", false
	}
	return service.origin, true
}

// loadTrustedServices translates the deployed typed settings and the optional
// trusted service-configuration file into one registry. Incomplete or
// conflicting configuration fails closed before any protocol frame is served.
func loadTrustedServices(lookup LookupEnv) (*trustedServices, error) {
	result := &trustedServices{
		byKey: map[string]trustedService{},
		sbiConfig: sbi.Config{
			Services:            map[string]sbi.ServiceConfig{},
			HeaderTokenProfiles: map[string]sbi.HeaderTokenProfile{},
		},
	}

	addSBI := func(key, origin, apiRoot, tokenProfile, token string) error {
		if _, duplicate := result.byKey[key]; duplicate {
			return fmt.Errorf("trusted service %q is configured more than once", key)
		}
		result.byKey[key] = trustedService{
			transport:          transportSBIH2C,
			origin:             origin,
			apiRoot:            apiRoot,
			headerTokenProfile: tokenProfile,
		}
		result.sbiConfig.Services[key] = sbi.ServiceConfig{
			Origin:             origin,
			APIRoot:            apiRoot,
			HeaderTokenProfile: tokenProfile,
		}
		if tokenProfile != "" {
			result.sbiConfig.HeaderTokenProfiles[tokenProfile] = sbi.HeaderTokenProfile{
				Header: queryTokenHeader,
				Token:  token,
			}
		}
		return nil
	}
	addMetrics := func(key, origin string) error {
		if _, duplicate := result.byKey[key]; duplicate {
			return fmt.Errorf("trusted service %q is configured more than once", key)
		}
		result.byKey[key] = trustedService{transport: transportMetricsHTTP1, origin: origin}
		// Metrics share the one transport instance with an explicit HTTP/1.1
		// trusted profile so connection lifecycle, cancellation, timeout,
		// redirect and size bounds are not duplicated.
		result.sbiConfig.Services[key] = sbi.ServiceConfig{
			Origin:  origin,
			Profile: sbi.TransportMetricsHTTP1,
		}
		return nil
	}

	sbiOrigin, sbiConfigured := lookup(EnvSMFSBIOrigin)
	queryToken, tokenConfigured := lookup(EnvQueryToken)
	if sbiConfigured != tokenConfigured {
		return nil, fmt.Errorf("SMF SBI origin and query token must be configured together")
	}
	if sbiConfigured {
		if err := addSBI(ServiceKeySMFContext, sbiOrigin, "", tokenProfileOpenAMSMQuery, queryToken); err != nil {
			return nil, err
		}
	}
	if amfOrigin, ok := lookup(EnvAMFMetricsOrigin); ok {
		if err := addMetrics(ServiceKeyAMFMetrics, amfOrigin); err != nil {
			return nil, err
		}
	}
	if smfMetricsOrigin, ok := lookup(EnvSMFMetricsOrigin); ok {
		if err := addMetrics(ServiceKeySMFMetrics, smfMetricsOrigin); err != nil {
			return nil, err
		}
	}

	if path, ok := lookup(EnvServiceConfig); ok && path != "" {
		document, err := serviceconfig.LoadDocument(path, lookup)
		if err != nil {
			return nil, fmt.Errorf("trusted service configuration is invalid: %w", err)
		}
		for key, service := range document.Config.Services {
			if _, duplicate := result.byKey[key]; duplicate {
				return nil, fmt.Errorf("trusted service %q is configured more than once", key)
			}
			result.byKey[key] = trustedService{
				transport:          transportSBIH2C,
				origin:             service.Origin,
				apiRoot:            service.APIRoot,
				headerTokenProfile: service.HeaderTokenProfile,
			}
			result.sbiConfig.Services[key] = service
		}
		for name, profile := range document.Config.HeaderTokenProfiles {
			if existing, duplicate := result.sbiConfig.HeaderTokenProfiles[name]; duplicate && existing != profile {
				return nil, fmt.Errorf("header token profile %q is configured more than once", name)
			}
			result.sbiConfig.HeaderTokenProfiles[name] = profile
		}
		if document.Config.Limits != (sbi.Limits{}) {
			result.sbiConfig.Limits = document.Config.Limits
		}
	}

	if len(result.byKey) == 0 {
		return nil, fmt.Errorf("no trusted OpenAMSM service is configured")
	}
	// Validate every h2c service and token profile through the shared transport
	// constructor once at initialization; the runtime builds its client from the
	// same normalized configuration.
	if len(result.sbiConfig.Services) > 0 {
		validationClient, err := sbi.NewClient(result.sbiConfig)
		if err != nil {
			return nil, fmt.Errorf("trusted SBI service configuration is invalid")
		}
		validationClient.CloseIdleConnections()
	}
	return result, nil
}
