#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "core/ogs-core.h"
#include "nas/5gs/ogs-nas-5gs.h"

#define INPUT_CAPACITY 4096
#define HEADROOM 128

static int hex_value(int ch)
{
    if (ch >= '0' && ch <= '9')
        return ch - '0';
    if (ch >= 'a' && ch <= 'f')
        return ch - 'a' + 10;
    if (ch >= 'A' && ch <= 'F')
        return ch - 'A' + 10;
    return -1;
}

static int read_hex_file(const char *path, unsigned char *out,
        size_t capacity, size_t *out_len)
{
    FILE *file = NULL;
    int ch = 0, high = -1;
    size_t length = 0;

    file = fopen(path, "rb");
    if (!file) {
        fprintf(stderr, "cannot open fixture: %s\n", path);
        return -1;
    }

    while ((ch = fgetc(file)) != EOF) {
        int value;

        if (isspace((unsigned char)ch))
            continue;
        value = hex_value(ch);
        if (value < 0) {
            fprintf(stderr, "non-hex character in fixture: %s\n", path);
            fclose(file);
            return -1;
        }
        if (high < 0) {
            high = value;
            continue;
        }
        if (length >= capacity) {
            fprintf(stderr, "fixture exceeds %u bytes\n",
                    (unsigned int)capacity);
            fclose(file);
            return -1;
        }
        out[length++] = (unsigned char)((high << 4) | value);
        high = -1;
    }

    fclose(file);
    if (high >= 0 || length == 0) {
        fprintf(stderr, "fixture has an odd or empty hex payload: %s\n", path);
        return -1;
    }

    *out_len = length;
    return 0;
}

static int print_flows(ogs_nas_qos_flow_descriptions_t *wire,
        const char *field_name, int *printed)
{
    ogs_nas_qos_flow_description_t flows[OGS_NAS_MAX_NUM_OF_QOS_FLOW_DESCRIPTION];
    int count = 0, i, j;

    if (!wire || wire->length == 0 || wire->buffer == NULL) {
        fprintf(stderr, "OpenAMSM decoded no %s IE\n", field_name);
        return -1;
    }

    memset(flows, 0, sizeof(flows));
    count = ogs_nas_parse_qos_flow_descriptions(flows, wire);
    if (count <= 0 || count > OGS_NAS_MAX_NUM_OF_QOS_FLOW_DESCRIPTION) {
        fprintf(stderr, "OpenAMSM QoS flow parser rejected %s IE\n", field_name);
        return -1;
    }

    printf("\"flows\":[");
    for (i = 0; i < count; i++) {
        ogs_nas_qos_flow_description_t *flow = &flows[i];

        if (i)
            putchar(',');
        printf("{\"qfi\":%u,\"operation\":%u,\"e\":%u,\"parameters\":[",
                (unsigned int)flow->identifier,
                (unsigned int)flow->code,
                (unsigned int)flow->E_bit);

        for (j = 0; j < flow->num_of_parameter; j++) {
            ogs_nas_qos_flow_parameter_t *parameter = &flow->param[j];

            if (j)
                putchar(',');
            if (parameter->identifier == OGS_NAX_QOS_FLOW_PARAMETER_ID_5QI) {
                printf("{\"id\":%u,\"length\":%u,\"fiveQI\":%u}",
                        (unsigned int)parameter->identifier,
                        (unsigned int)parameter->len,
                        (unsigned int)parameter->qos_index);
            } else {
                printf("{\"id\":%u,\"length\":%u,\"unit\":%u,\"value\":%u,\"bps\":%llu}",
                        (unsigned int)parameter->identifier,
                        (unsigned int)parameter->len,
                        (unsigned int)parameter->br.unit,
                        (unsigned int)parameter->br.value,
                        (unsigned long long)ogs_nas_bitrate_to_uint64(&parameter->br));
            }
        }
        printf("]}");
    }
    putchar(']');
    *printed = 1;
    return 0;
}

int main(int argc, char **argv)
{
    unsigned char storage[INPUT_CAPACITY + HEADROOM];
    unsigned char *input = storage + HEADROOM;
    size_t input_len = 0;
    ogs_pkbuf_t pkbuf;
    ogs_nas_5gs_message_t message;
    ogs_nas_qos_flow_descriptions_t *descriptions = NULL;
    int status = 1, core_initialized = 0, flows_printed = 0;
    unsigned int message_type = 0;

    if (argc != 2) {
        fprintf(stderr, "usage: %s <nas-hex-fixture>\n", argv[0]);
        return 2;
    }

    if (read_hex_file(argv[1], input, INPUT_CAPACITY, &input_len) != 0)
        return 2;

    memset(&pkbuf, 0, sizeof(pkbuf));
    memset(&message, 0, sizeof(message));

    /*
     * ogs_nas_5gsm_decode() uses only the packet-buffer cursor operations for
     * this unprotected NAS payload. Give it real headroom because its decoder
     * restores the consumed bytes with ogs_pkbuf_push().
     */
    pkbuf.head = storage;
    pkbuf.data = input;
    pkbuf.tail = input + input_len;
    pkbuf.end = pkbuf.tail;
    pkbuf.len = (unsigned int)input_len;

    ogs_core_initialize();
    core_initialized = 1;
    ogs_log_install_domain(&__ogs_nas_domain, "nas-oracle", OGS_LOG_ERROR);

    if (ogs_nas_5gsm_decode(&message, &pkbuf) != OGS_OK) {
        fprintf(stderr, "OpenAMSM ogs_nas_5gsm_decode() rejected fixture\n");
        goto cleanup;
    }

    if (message.gsm.h.extended_protocol_discriminator !=
            OGS_NAS_EXTENDED_PROTOCOL_DISCRIMINATOR_5GSM) {
        fprintf(stderr, "OpenAMSM decoded an unexpected EPD\n");
        goto cleanup;
    }

    message_type = message.gsm.h.message_type;
    printf("{\"messageType\":%u,\"psi\":%u,\"pti\":%u,",
            message_type,
            (unsigned int)message.gsm.h.pdu_session_identity,
            (unsigned int)message.gsm.h.procedure_transaction_identity);

    switch (message_type) {
    case OGS_NAS_5GS_PDU_SESSION_MODIFICATION_REQUEST:
        descriptions =
            &message.gsm.pdu_session_modification_request.requested_qos_flow_descriptions;
        if (print_flows(descriptions, "Requested QoS flow descriptions",
                &flows_printed) != 0)
            goto cleanup;
        break;

    case OGS_NAS_5GS_PDU_SESSION_MODIFICATION_COMMAND:
        descriptions =
            &message.gsm.pdu_session_modification_command.authorized_qos_flow_descriptions;
        if (print_flows(descriptions, "Authorized QoS flow descriptions",
                &flows_printed) != 0)
            goto cleanup;
        break;

    case OGS_NAS_5GS_PDU_SESSION_MODIFICATION_COMPLETE:
        if (input_len != sizeof(ogs_nas_5gsm_header_t)) {
            fprintf(stderr, "expected the scoped header-only 0xCC fixture\n");
            goto cleanup;
        }
        printf("\"flows\":[]");
        flows_printed = 1;
        break;

    default:
        fprintf(stderr, "unexpected NAS message type 0x%02x\n", message_type);
        goto cleanup;
    }

    if (!flows_printed) {
        fprintf(stderr, "OpenAMSM oracle did not emit decoded fields\n");
        goto cleanup;
    }

    printf("}\n");
    status = 0;

cleanup:
    if (core_initialized)
        ogs_core_terminate();
    return status;
}
