package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"unicode/utf8"

	"github.com/odango-chan/ca-client-tools/internal/openamsm/metrics"
	smfcontext "github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	Version              = "dev"
	maxContextInputBytes = 64 << 10
	exitInvalid          = 2
	exitConfiguration    = 3
	exitRemote           = 4
	exitInternal         = 5
)

type lookupEnv func(string) (string, bool)

type dependencies struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv lookupEnv
}

type dataEnvelope struct {
	Data any `json:"data"`
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Cause      string `json:"cause,omitempty"`
}

type commandError struct {
	detail   errorDetail
	exitCode int
}

func (e *commandError) Error() string {
	return e.detail.Code
}

func invalidArgument() *commandError {
	return &commandError{detail: errorDetail{Code: "INVALID_ARGUMENT"}, exitCode: exitInvalid}
}

func missingConfiguration() *commandError {
	return &commandError{detail: errorDetail{Code: "CONFIGURATION_MISSING"}, exitCode: exitConfiguration}
}

func invalidConfiguration() *commandError {
	return &commandError{detail: errorDetail{Code: "CONFIGURATION_INVALID"}, exitCode: exitConfiguration}
}

func internalFailure() *commandError {
	return &commandError{detail: errorDetail{Code: "INTERNAL_ERROR"}, exitCode: exitInternal}
}

// Run is the executable boundary. It injects process streams and trusted
// environment lookup, and propagates interrupt/SIGTERM cancellation to the
// command context. os.Exit remains in cmd/ca-client-tools/main.go.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.Signal(15))
	defer signal.Stop(signals)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signalExit := make(chan int, 1)
	go func() {
		select {
		case received := <-signals:
			signalExit <- signalExitCode(received)
			cancel()
		case <-ctx.Done():
		}
	}()

	code := runWithContext(ctx, args, stdin, stdout, stderr, os.LookupEnv)
	select {
	case receivedCode := <-signalExit:
		return receivedCode
	default:
		return code
	}
}

func signalExitCode(received os.Signal) int {
	if value, ok := received.(syscall.Signal); ok {
		return 128 + int(value)
	}
	return 130
}

func runWithContext(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	getenv lookupEnv,
) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if getenv == nil {
		getenv = os.LookupEnv
	}

	deps := dependencies{stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv}
	root := newRootCommand(deps)
	root.SetArgs(args)

	err := root.ExecuteContext(ctx)
	if ctx.Err() != nil {
		// A process signal or Host cancellation is incomplete process control,
		// not a remote business result. Do not emit either JSON envelope.
		return 130
	}
	if err == nil {
		return 0
	}

	var commandErr *commandError
	if errors.As(err, &commandErr) {
		return emitError(stderr, commandErr)
	}
	return emitError(stderr, invalidArgument())
}

func newRootCommand(deps dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "ca-client-tools",
		Short:         "Local OpenAMSM client tools",
		Long:          "A non-interactive, machine-readable client for the approved OpenAMSM read interfaces.",
		Version:       Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, _ error) error {
		return invalidArgument()
	})
	root.AddCommand(newAMFCommand(deps), newSMFCommand(deps), newProviderCommand(deps))
	return root
}

func newAMFCommand(deps dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "amf",
		Short: "AMF read-only operations",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	ue := &cobra.Command{
		Use:   "ue",
		Short: "UE resources",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	gnb := &cobra.Command{
		Use:   "gnb",
		Short: "gNB resources",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	ue.AddCommand(newMetricsListCommand(
		deps,
		"List one page of AMF UE information",
		"CA_OPENAMSM_AMF_METRICS_ORIGIN",
		func(origin string) metrics.Config { return metrics.Config{AMFOrigin: origin} },
		func(client *metrics.Client, ctx context.Context, page, pageSize int) (metrics.Page, error) {
			return client.GetUEInfo(ctx, page, pageSize)
		},
	))
	gnb.AddCommand(newMetricsListCommand(
		deps,
		"List one page of AMF gNB information",
		"CA_OPENAMSM_AMF_METRICS_ORIGIN",
		func(origin string) metrics.Config { return metrics.Config{AMFOrigin: origin} },
		func(client *metrics.Client, ctx context.Context, page, pageSize int) (metrics.Page, error) {
			return client.GetGNBInfo(ctx, page, pageSize)
		},
	))
	command.AddCommand(ue, gnb)
	return command
}

func newSMFCommand(deps dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "smf",
		Short: "SMF read-only operations",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	pdu := &cobra.Command{
		Use:   "pdu",
		Short: "PDU Session resources",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	contextGroup := &cobra.Command{
		Use:   "context",
		Short: "SMF context query operations",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	pdu.AddCommand(newMetricsListCommand(
		deps,
		"List one page of SMF PDU Session information",
		"CA_OPENAMSM_SMF_METRICS_ORIGIN",
		func(origin string) metrics.Config { return metrics.Config{SMFOrigin: origin} },
		func(client *metrics.Client, ctx context.Context, page, pageSize int) (metrics.Page, error) {
			return client.GetPDUInfo(ctx, page, pageSize)
		},
	))
	contextGroup.AddCommand(newSMFContextQueryCommand(deps))
	command.AddCommand(pdu, contextGroup)
	return command
}

func showHelp(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

type metricsConfig func(string) metrics.Config

type metricsCall func(*metrics.Client, context.Context, int, int) (metrics.Page, error)

func newMetricsListCommand(
	deps dependencies,
	short string,
	originEnv string,
	config metricsConfig,
	call metricsCall,
) *cobra.Command {
	return newMetricsListCommandWithDefaultPageSize(deps, short, originEnv, config, call, 100)
}

func newMetricsListCommandWithDefaultPageSize(
	deps dependencies,
	short string,
	originEnv string,
	config metricsConfig,
	call metricsCall,
	defaultPageSize int,
) *cobra.Command {
	page := 0
	pageSize := defaultPageSize
	pageValue := &onceIntValue{target: &page}
	pageSizeValue := &onceIntValue{target: &pageSize}

	command := &cobra.Command{
		Use:   "list",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			origin, ok := deps.getenv(originEnv)
			if !ok {
				return missingConfiguration()
			}
			client, err := metrics.NewClient(config(origin))
			if err != nil {
				return mapMetricsError(err)
			}
			result, err := call(client, cmd.Context(), page, pageSize)
			if err != nil {
				if cmd.Context().Err() != nil {
					return err
				}
				return mapMetricsError(err)
			}
			return emitData(cmd.Context(), cmd.OutOrStdout(), result)
		},
	}
	command.Flags().Var(pageValue, "page", "Zero-based page index (default 0)")
	command.Flags().Var(pageSizeValue, "page-size", "Page size from 1 to 100 (default "+strconv.Itoa(defaultPageSize)+")")
	return command
}

func newSMFContextQueryCommand(deps dependencies) *cobra.Command {
	var contextType string
	contextTypeValue := &onceStringValue{target: &contextType}
	command := &cobra.Command{
		Use:   "query",
		Short: "Query one SMF context projection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, ok := openAMSMContextTypeID[contextType]; !ok {
				return invalidArgument()
			}
			origin, originOK := deps.getenv("CA_OPENAMSM_SMF_SBI_ORIGIN")
			token, tokenOK := deps.getenv("CA_OPENAMSM_QUERY_TOKEN")
			if !originOK || !tokenOK {
				return missingConfiguration()
			}

			query, err := readSMFContextQuery(deps.stdin, contextType)
			if err != nil {
				return err
			}
			client, err := smfcontext.NewClient(smfcontext.Config{Origin: origin, QueryToken: token})
			if err != nil {
				return mapSMFContextError(err)
			}
			result, err := client.Query(cmd.Context(), query)
			if err != nil {
				if cmd.Context().Err() != nil {
					return err
				}
				return mapSMFContextError(err)
			}
			return emitData(cmd.Context(), cmd.OutOrStdout(), publicSMFContextResult{
				Type: contextType, Status: result.Status, Key: result.Key, Context: result.Context,
			})
		},
	}
	command.Flags().Var(contextTypeValue, "type", "SMF context type: session, policy, up-node, signaling-session, ip-address, up-selection, charging or session-auth")
	if err := command.MarkFlagRequired("type"); err != nil {
		panic(err)
	}
	return command
}

// openAMSMContextTypeID is the private wire mapping. Its keys are the stable
// CA-facing context types; its values must never be exposed by the CLI.
var openAMSMContextTypeID = map[string]string{
	"session":           "SC",
	"policy":            "PA",
	"up-node":           "UPN",
	"signaling-session": "SSM",
	"ip-address":        "IAM",
	"up-selection":      "UPS",
	"charging":          "CA",
	"session-auth":      "SAA",
}

type publicSMFContextResult struct {
	Type    string         `json:"type"`
	Status  string         `json:"status"`
	Key     map[string]any `json:"key"`
	Context map[string]any `json:"context"`
}

func readSMFContextQuery(input io.Reader, contextType string) (smfcontext.Query, error) {
	masID, ok := openAMSMContextTypeID[contextType]
	if !ok {
		return smfcontext.Query{}, invalidArgument()
	}
	raw, err := io.ReadAll(io.LimitReader(input, maxContextInputBytes+1))
	if err != nil {
		return smfcontext.Query{}, internalFailure()
	}
	if len(raw) > maxContextInputBytes || !utf8.Valid(raw) {
		return smfcontext.Query{}, invalidArgument()
	}

	decoded, err := decodeUniqueJSON(raw)
	if err != nil {
		return smfcontext.Query{}, invalidArgument()
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return smfcontext.Query{}, invalidArgument()
	}

	allowed := map[string]struct{}{
		"supi": {}, "imsi": {}, "pduSessionId": {}, "ueIpv4": {}, "ueIpv6": {},
	}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return smfcontext.Query{}, invalidArgument()
		}
	}

	query := smfcontext.Query{MAS: masID}
	if value, exists := object["supi"]; exists {
		text, ok := value.(string)
		if !ok {
			return smfcontext.Query{}, invalidArgument()
		}
		query.SUPI = text
	}
	if value, exists := object["imsi"]; exists {
		text, ok := value.(string)
		if !ok {
			return smfcontext.Query{}, invalidArgument()
		}
		query.IMSI = text
	}
	if value, exists := object["ueIpv4"]; exists {
		text, ok := value.(string)
		if !ok {
			return smfcontext.Query{}, invalidArgument()
		}
		query.UEIPv4 = text
	}
	if value, exists := object["ueIpv6"]; exists {
		text, ok := value.(string)
		if !ok {
			return smfcontext.Query{}, invalidArgument()
		}
		query.UEIPv6 = text
	}
	if value, exists := object["pduSessionId"]; exists {
		number, ok := value.(json.Number)
		if !ok {
			return smfcontext.Query{}, invalidArgument()
		}
		parsed, err := number.Int64()
		if err != nil || parsed < 0 || parsed > 255 {
			return smfcontext.Query{}, invalidArgument()
		}
		psi := int(parsed)
		query.PDUSessionID = &psi
	}
	return query, nil
}

func decodeUniqueJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}

	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, errors.New("duplicate JSON object key")
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, errors.New("unterminated JSON object")
		}
		return object, nil
	case '[':
		values := make([]any, 0)
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, errors.New("unterminated JSON array")
		}
		return values, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

func mapMetricsError(err error) *commandError {
	var inputErr *metrics.InputError
	if errors.As(err, &inputErr) {
		return invalidArgument()
	}
	var configErr *metrics.ConfigurationError
	if errors.As(err, &configErr) {
		return invalidConfiguration()
	}
	var remoteErr *metrics.RemoteError
	if errors.As(err, &remoteErr) {
		return &commandError{
			detail:   errorDetail{Code: "OPENAMSM_HTTP_ERROR", HTTPStatus: remoteErr.HTTPStatus},
			exitCode: exitRemote,
		}
	}
	var protocolErr *metrics.ProtocolError
	if errors.As(err, &protocolErr) {
		return &commandError{detail: errorDetail{Code: "OPENAMSM_PROTOCOL_INVALID"}, exitCode: exitInternal}
	}
	return &commandError{detail: errorDetail{Code: "OPENAMSM_TRANSPORT_ERROR"}, exitCode: exitRemote}
}

func mapSMFContextError(err error) *commandError {
	var inputErr *smfcontext.InputError
	if errors.As(err, &inputErr) {
		return invalidArgument()
	}
	var configErr *smfcontext.ConfigurationError
	if errors.As(err, &configErr) {
		return invalidConfiguration()
	}
	var remoteErr *smfcontext.RemoteError
	if errors.As(err, &remoteErr) {
		return &commandError{
			detail: errorDetail{
				Code:       "OPENAMSM_HTTP_ERROR",
				HTTPStatus: remoteErr.HTTPStatus,
				Cause:      remoteErr.Cause,
			},
			exitCode: exitRemote,
		}
	}
	var protocolErr *smfcontext.ProtocolError
	if errors.As(err, &protocolErr) {
		return &commandError{detail: errorDetail{Code: "OPENAMSM_PROTOCOL_INVALID"}, exitCode: exitInternal}
	}
	return &commandError{detail: errorDetail{Code: "OPENAMSM_TRANSPORT_ERROR"}, exitCode: exitRemote}
}

func emitData(ctx context.Context, output io.Writer, value any) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	encoded, err := json.Marshal(dataEnvelope{Data: value})
	if err != nil {
		return internalFailure()
	}
	encoded = append(encoded, '\n')
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if _, err := output.Write(encoded); err != nil {
		return internalFailure()
	}
	return nil
}

func emitError(output io.Writer, failure *commandError) int {
	if err := json.NewEncoder(output).Encode(errorEnvelope{Error: failure.detail}); err != nil {
		return exitInternal
	}
	return failure.exitCode
}

type onceStringValue struct {
	target *string
	set    bool
}

func (v *onceStringValue) Set(raw string) error {
	if v.set {
		return errors.New("flag may only be specified once")
	}
	v.set = true
	*v.target = raw
	return nil
}

func (v *onceStringValue) String() string {
	return *v.target
}

func (v *onceStringValue) Type() string {
	return "string"
}

type onceIntValue struct {
	target *int
	set    bool
}

func (v *onceIntValue) Set(raw string) error {
	if v.set {
		return errors.New("flag may only be specified once")
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	v.set = true
	*v.target = value
	return nil
}

func (v *onceIntValue) String() string {
	return strconv.Itoa(*v.target)
}

func (v *onceIntValue) Type() string {
	return "int"
}

var _ pflag.Value = (*onceStringValue)(nil)
var _ pflag.Value = (*onceIntValue)(nil)
