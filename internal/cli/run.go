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

func Run(args []string, stdout, stderr io.Writer) int {
	return runWithEnv(args, stdout, stderr, os.LookupEnv)
}

func runWithEnv(args []string, stdout, stderr io.Writer, getenv lookupEnv) int {
	if len(args) == 0 || isHelp(args) {
		writeUsage(stdout)
		return 0
	}
	if len(args) < 3 || args[0] != "smf" || args[1] != "mas" || args[2] != "query" {
		return writeError(stdout, errorDetail{Code: "COMMAND_UNSUPPORTED"})
	}
	return runSMFMASQuery(args[3:], stdout, stderr, getenv)
}

func runSMFMASQuery(args []string, stdout, stderr io.Writer, getenv lookupEnv) int {
	flags := flag.NewFlagSet("smf mas query", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	masID := flags.String("mas", "", "SMF MAS short name")
	supi := flags.String("supi", "", "SUPI in imsi-<15 digits> form")
	imsi := flags.String("imsi", "", "15-digit decimal IMSI")
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
		return writeClientError(stdout, err)
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintln(stderr, "ca-client-tools: output encoding failed")
		return 5
	}
	return 0
}

func writeClientError(stdout io.Writer, err error) int {
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
  ca-client-tools smf mas query --mas <SC|PA|UPN|SSM|IAM|UPS|CA|SAA> [lookup key]

Lookup keys:
  --supi imsi-<15 digits> --pdu-session-id <0..255>
  --imsi <15 digits> --pdu-session-id <0..255>
  --ue-ipv4 <address>
  --ue-ipv6 <address>

Trusted environment:
  CA_OPENAMSM_SMF_SBI_ORIGIN
  CA_OPENAMSM_QUERY_TOKEN

The command emits one JSON result to stdout. Errors are emitted as a
machine-readable JSON error envelope. The CLI never accepts an arbitrary
endpoint or query token as command-line arguments.`) + "\n")
}
