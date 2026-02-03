package tasks

import (
	"errors"

	"github.com/dianabuilds/ardents/internal/shared/codec"
)

const (
	RequestType  = "task.request.v1"
	AcceptType   = "task.accept.v1"
	ProgressType = "task.progress.v1"
	ResultType   = "task.result.v1"
	FailType     = "task.fail.v1"
	ReceiptType  = "task.receipt.v1"
	Version      = 1
)

var (
	ErrTaskUnsupported = errors.New("ERR_TASK_UNSUPPORTED")
	ErrTaskRejected    = errors.New("ERR_TASK_REJECTED")
	ErrTaskTimeout     = errors.New("ERR_TASK_TIMEOUT")
)

type Request struct {
	V               uint64         `cbor:"v"`
	TaskID          string         `cbor:"task_id"`
	ClientRequestID string         `cbor:"client_request_id"`
	JobType         string         `cbor:"job_type"`
	Input           map[string]any `cbor:"input"`
	TSMs            int64          `cbor:"ts_ms"`
}

type Accept struct {
	V      uint64 `cbor:"v"`
	TaskID string `cbor:"task_id"`
	TSMs   int64  `cbor:"ts_ms"`
}

type Progress struct {
	V      uint64 `cbor:"v"`
	TaskID string `cbor:"task_id"`
	Pct    uint64 `cbor:"pct"`
	Note   string `cbor:"note,omitempty"`
	TSMs   int64  `cbor:"ts_ms"`
}

type Result struct {
	V            uint64 `cbor:"v"`
	TaskID       string `cbor:"task_id"`
	ResultNodeID string `cbor:"result_node_id"`
	TSMs         int64  `cbor:"ts_ms"`
}

type Fail struct {
	V            uint64 `cbor:"v"`
	TaskID       string `cbor:"task_id"`
	ErrorCode    string `cbor:"error_code"`
	ErrorMessage string `cbor:"error_message,omitempty"`
	TSMs         int64  `cbor:"ts_ms"`
}

type Receipt struct {
	V       uint64           `cbor:"v"`
	TaskID  string           `cbor:"task_id"`
	Metrics map[string]int64 `cbor:"metrics"`
	TSMs    int64            `cbor:"ts_ms"`
}

func EncodeRequest(r Request) ([]byte, error) { return codec.Marshal(r) }
func DecodeRequest(b []byte) (Request, error) {
	var r Request
	return r, codec.Unmarshal(b, &r)
}

func EncodeAccept(r Accept) ([]byte, error) { return codec.Marshal(r) }
func DecodeAccept(b []byte) (Accept, error) {
	var r Accept
	return r, codec.Unmarshal(b, &r)
}

func EncodeProgress(r Progress) ([]byte, error) { return codec.Marshal(r) }
func DecodeProgress(b []byte) (Progress, error) {
	var r Progress
	return r, codec.Unmarshal(b, &r)
}

func EncodeResult(r Result) ([]byte, error) { return codec.Marshal(r) }
func DecodeResult(b []byte) (Result, error) {
	var r Result
	return r, codec.Unmarshal(b, &r)
}

func EncodeFail(r Fail) ([]byte, error) { return codec.Marshal(r) }
func DecodeFail(b []byte) (Fail, error) {
	var r Fail
	return r, codec.Unmarshal(b, &r)
}

func EncodeReceipt(r Receipt) ([]byte, error) { return codec.Marshal(r) }
func DecodeReceipt(b []byte) (Receipt, error) {
	var r Receipt
	return r, codec.Unmarshal(b, &r)
}

var (
	_ = ProgressType
	_ = ReceiptType
	_ = ErrTaskTimeout
	_ = DecodeAccept
	_ = EncodeProgress
	_ = DecodeProgress
	_ = EncodeReceipt
	_ = DecodeReceipt
)
