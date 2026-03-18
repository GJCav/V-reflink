package protocol

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"

	"github.com/KarpelesLab/reflink"
	"golang.org/x/sys/unix"
)

const (
	Version1  = 1
	Version2  = 2
	OpReflink = "reflink"
)

const (
	CodeENOENT     = "ENOENT"
	CodeEEXIST     = "EEXIST"
	CodeEOPNOTSUPP = "EOPNOTSUPP"
	CodeEINVAL     = "EINVAL"
	CodeEPERM      = "EPERM"
	defaultErrCode = CodeEINVAL
	defaultErrMsg  = "request failed"
)

type Request struct {
	Version   int    `json:"version"`
	Op        string `json:"op"`
	Recursive bool   `json:"recursive"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Token     string `json:"token,omitempty"`
}

type Response struct {
	OK    bool         `json:"ok"`
	Error *ErrorDetail `json:"error,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CodedError struct {
	Code    string
	Message string
	Err     error
}

func (e *CodedError) Error() string {
	return e.Message
}

func (e *CodedError) Unwrap() error {
	return e.Err
}

func (r Request) Validate() error {
	switch {
	case r.Version != Version1 && r.Version != Version2:
		return NewError(CodeEINVAL, fmt.Sprintf("unsupported protocol version: %d", r.Version))
	case r.Op != OpReflink:
		return NewError(CodeEINVAL, fmt.Sprintf("unsupported operation: %s", r.Op))
	case r.Version == Version1 && r.Token != "":
		return NewError(CodeEINVAL, "authentication token requires protocol version 2")
	case r.Version == Version2 && r.Token == "":
		return NewError(CodeEINVAL, "authentication token is required")
	case r.Src == "":
		return NewError(CodeEINVAL, "source path is required")
	case r.Dst == "":
		return NewError(CodeEINVAL, "destination path is required")
	case r.Src == r.Dst:
		return NewError(CodeEINVAL, "source and destination must differ")
	default:
		return nil
	}
}

func SuccessResponse() Response {
	return Response{OK: true}
}

func FailureResponse(code, message string) Response {
	return Response{
		OK: false,
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
}

func NewError(code, message string) error {
	return &CodedError{Code: code, Message: message}
}

func WrapError(code, message string, err error) error {
	return &CodedError{Code: code, Message: message, Err: err}
}

func ResponseFromError(err error) Response {
	if err == nil {
		return SuccessResponse()
	}

	if coded, ok := AsCoded(err); ok {
		return FailureResponse(coded.Code, coded.Message)
	}

	return FailureResponse(CodeFromError(err), MessageFromError(err))
}

func AsCoded(err error) (*CodedError, bool) {
	var coded *CodedError
	if errors.As(err, &coded) {
		return coded, true
	}

	return nil, false
}

func CodeFromError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.ENOTDIR):
		return CodeENOENT
	case errors.Is(err, fs.ErrExist), errors.Is(err, syscall.EEXIST):
		return CodeEEXIST
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EACCES):
		return CodeEPERM
	case errors.Is(err, reflink.ErrReflinkUnsupported), errors.Is(err, reflink.ErrReflinkFailed),
		errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.ENOTSUP), errors.Is(err, syscall.EXDEV):
		return CodeEOPNOTSUPP
	default:
		return defaultErrCode
	}
}

func MessageFromError(err error) string {
	if coded, ok := AsCoded(err); ok {
		return coded.Message
	}

	switch CodeFromError(err) {
	case CodeENOENT:
		return "source not found"
	case CodeEEXIST:
		return "destination already exists"
	case CodeEOPNOTSUPP:
		return "reflink is not supported for this source and destination"
	case CodeEPERM:
		return "permission denied"
	case CodeEINVAL:
		if err != nil {
			return err.Error()
		}
	}

	return defaultErrMsg
}
