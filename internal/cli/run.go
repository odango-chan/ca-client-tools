package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/odango-chan/ca-client-tools/internal/openamsm/metrics"
	"github.com/odango-chan/ca-client-tools/internal/openamsm/smf/mas"
)

type lookupEnv func(string) (string, bool)

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Cause      string `json:"cause,omitempty"`
}

type metricsQuery func(*metrics.Client, context.Context, int, int) (metrics.Page, error)

func Run(args []string, stdout, stderr io.Writer) int {
	return runWithEnv(args, stdout, stderr, os.LookupEnv)
}

func runWithEnv(args []string, stdout, stderr io.Writer, getenv lookupEnv) int {
	if len(args) == 0 || isHelp(args) {
		writeUsage(stdout)
		return 0
	}
	switch {
	case len(args) >= 3 && args[0] == "smf" && args[1] == "mas" && args[2] == "query":
		return runSMFMASQuery(args[3:], stdout, stderr, getenv)
	case len(args) >= 2 && args[0] == "amf" && args[1] == "ue-info":
		return runMetricsQuery(args[2:], "CA_OPENAMSM_AMF_METRICS_ORIGIN", true, stdout, stderr, getenv,
			func(client *metrics.Client, ctx context.Context, page, size int) (metrics.Page, error) {
				return client.GetUEInfo(ctx, page, size)
			})
	case len(args) >= 2 && args[0] == "amf" && args[1] == "gnb-info":
		return runMetricsQuery(args[2:], "CA_OPENAMSM_AMF_METRICS_ORIGIN", true, stdout, stderr, getenv,
			func(client *metrics.Client, ctx context.Context, page, size int) (metrics.Page, error) {
				return client.GetGNBInfo(ctx, page, size)
			})
	case len(args) >= 2 && args[0] == "smf" && args[1] == "pdu-info":
		return runMetricsQuery(args[2:], "CA_OPENAMSM_SMF_METRICS_ORIGIN", false, stdout, stderr, getenv,
			func(client *metrics.Client, ctx context.Context, page, size int) (metrics.Page, error) {
				return client.GetPDUInfo(ctx, page, size)
			})
	default:
		return writeError(stdout, errorDetail{Code: "COMMAND_UNSUPPORTED"})
	}
}

func runMetricsQuery(args []string, originEnv string, isAMF bool, stdout, stderr io.Writer, getenv lookupEnv, query metricsQuery) int {
	flags := flag.NewFlagSet("OpenAMSM metrics query", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	page := flags.Int("page", 0, "zero-based page index")
	pageSize := flags.Int("page-size", 100, "page size (1-100)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return 0
		}
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}
	if flags.NArg() != 0 {
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}
	origin, ok := getenv(originEnv)
	if !ok || origin == "" {
		return writeError(stdout, errorDetail{Code: "CONFIGURATION_MISSING"})
	}
	cfg := metrics.Config{}
	if isAMF {
		cfg.AMFOrigin = origin
	} else {
		cfg.SMFOrigin = origin
	}
	client, err := metrics.NewClient(cfg)
	if err != nil {
		return writeMetricsError(stdout, err)
	}
	result, err := query(client, context.Background(), *page, *pageSize)
	if err != nil {
		return writeMetricsError(stdout, err)
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintln(stderr, "ca-client-tools: output encoding failed")
		return 5
	}
	return 0
}

func runSMFMASQuery(args []string, stdout, stderr io.Writer, getenv lookupEnv) int {
	flags := flag.NewFlagSet("smf mas query", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	masID := flags.String("mas", "", "SMF MAS short name")
	supi := flags.String("supi", "", "SUPI in imsi-<1..15 digits> form")
	imsi := flags.String("imsi", "", "1..15 decimal IMSI digits")
	psi := flags.Int("pdu-session-id", -1, "PDU Session ID (0-255)")
	ueIPv4 := flags.String("ue-ipv4", "", "UE IPv4 lookup key")
	ueIPv6 := flags.String("ue-ipv6", "", "UE IPv6 lookup key")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return 0
		}
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}
	if flags.NArg() != 0 {
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}

	origin, originOK := getenv("CA_OPENAMSM_SMF_SBI_ORIGIN")
	token, tokenOK := getenv("CA_OPENAMSM_QUERY_TOKEN")
	if !originOK || !tokenOK {
		return writeError(stdout, errorDetail{Code: "CONFIGURATION_MISSING"})
	}

	client, err := mas.NewClient(mas.Config{Origin: origin, QueryToken: token})
	if err != nil {
		return writeError(stdout, errorDetail{Code: "CONFIGURATION_INVALID"})
	}
	query := mas.Query{
		MAS:    *masID,
		SUPI:   *supi,
		IMSI:   *imsi,
		UEIPv4: *ueIPv4,
		UEIPv6: *ueIPv6,
	}
	if *psi >= 0 {
		value := *psi
		query.PDUSessionID = &value
	} else if *psi != -1 {
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}

	result, err := client.Query(context.Background(), query)
	if err != nil {
		return writeMASClientError(stdout, err)
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintln(stderr, "ca-client-tools: output encoding failed")
		return 5
	}
	return 0
}

func writeMASClientError(stdout io.Writer, err error) int {
	var inputErr *mas.InputError
	if errors.As(err, &inputErr) {
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}
	var configErr *mas.ConfigurationError
	if errors.As(err, &configErr) {
		return writeError(stdout, errorDetail{Code: "CONFIGURATION_INVALID"})
	}
	var protocolErr *mas.ProtocolError
	if errors.As(err, &protocolErr) {
		return writeError(stdout, errorDetail{Code: "OPENAMSM_PROTOCOL_INVALID"})
	}
	var remoteErr *mas.RemoteError
	if errors.As(err, &remoteErr) {
		return writeError(stdout, errorDetail{
			Code: "OPENAMSM_HTTP_ERROR", HTTPStatus: remoteErr.HTTPStatus, Cause: remoteErr.Cause,
		})
	}
	return writeError(stdout, errorDetail{Code: "OPENAMSM_TRANSPORT_ERROR"})
}

func writeMetricsError(stdout io.Writer, err error) int {
	var inputErr *metrics.InputError
	if errors.As(err, &inputErr) {
		return writeError(stdout, errorDetail{Code: "INVALID_ARGUMENT"})
	}
	var configErr *metrics.ConfigurationError
	if errors.As(err, &configErr) {
		return writeError(stdout, errorDetail{Code: "CONFIGURATION_INVALID"})
	}
	var protocolErr *metrics.ProtocolError
	if errors.As(err, &protocolErr) {
		return writeError(stdout, errorDetail{Code: "OPENAMSM_PROTOCOL_INVALID"})
	}
	var remoteErr *metrics.RemoteError
	if errors.As(err, &remoteErr) {
		return writeError(stdout, errorDetail{Code: "OPENAMSM_HTTP_ERROR", HTTPStatus: remoteErr.HTTPStatus})
	}
	return writeError(stdout, errorDetail{Code: "OPENAMSM_TRANSPORT_ERROR"})
}

func writeError(stdout io.Writer, detail errorDetail) int {
	_ = json.NewEncoder(stdout).Encode(errorEnvelope{Error: detail})
	switch detail.Code {
	case "INVALID_ARGUMENT", "COMMAND_UNSUPPORTED":
		return 2
	case "CONFIGURATION_MISSING", "CONFIGURATION_INVALID":
		return 3
	case "OPENAMSM_HTTP_ERROR", "OPENAMSM_TRANSPORT_ERROR":
		return 4
	default:
		return 5
	}
}

func isHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}

func writeUsage(writer io.Writer) {
	_, _ = io.WriteString(writer, strings.TrimSpace(`Usage:
  ca-client-tools amf ue-info [--page 0] [--page-size 100]
  ca-client-tools amf gnb-info [--page 0] [--page-size 100]
  ca-client-tools smf pdu-info [--page 0] [--page-size 100]
  ca-client-tools smf mas query --mas <SC|PA|UPN|SSM|IAM|UPS|CA|SAA> [lookup key]

Metrics lookup:
  CA_OPENAMSM_AMF_METRICS_ORIGIN
  CA_OPENAMSM_SMF_METRICS_ORIGIN

SMF MAS lookup keys:
  --supi imsi-<1..15 digits> and/or --imsi <1..15 digits>
    with --pdu-session-id <0..255>
  --ue-ipv4 <address> and/or --ue-ipv6 <address>
  Identity+PSI may be combined with either or both UE IP keys.

SMF MAS trusted configuration:
  CA_OPENAMSM_SMF_SBI_ORIGIN
  CA_OPENAMSM_QUERY_TOKEN

Metrics commands return one validated source page. Pages are zero-based and
page-size is limited to 100. The Host controls pagination and Agent visibility.
The CLI never accepts an arbitrary endpoint or query token as command-line
arguments.`)+"\n")
}
