package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/GJCav/V-reflink/internal/auth"
	"github.com/GJCav/V-reflink/internal/framing"
	"github.com/GJCav/V-reflink/internal/protocol"
)

type stubRequestService struct {
	requests []protocol.Request
	err      error
}

func (s *stubRequestService) Execute(req protocol.Request) error {
	s.requests = append(s.requests, req)
	return s.err
}

type stubWorkerInvoker struct {
	requests   []protocol.Request
	identities []auth.Identity
	err        error
}

func (w *stubWorkerInvoker) Execute(_ context.Context, req protocol.Request, identity auth.Identity) error {
	w.requests = append(w.requests, req)
	w.identities = append(w.identities, identity)
	return w.err
}

func TestDaemonExecutorWithoutTokenMapUsesDirectService(t *testing.T) {
	service := &stubRequestService{}
	executor := daemonExecutor{service: service}

	req := protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
	}

	if err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(service.requests) != 1 || service.requests[0] != req {
		t.Fatalf("service requests = %#v, want %#v", service.requests, []protocol.Request{req})
	}
}

func TestDaemonExecutorWithoutTokenMapRejectsTokenAuth(t *testing.T) {
	service := &stubRequestService{}
	executor := daemonExecutor{service: service}

	err := executor.Execute(context.Background(), protocol.Request{
		Version: protocol.Version2,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
		Token:   "token-a",
	})
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	coded, ok := protocol.AsCoded(err)
	if !ok {
		t.Fatalf("Execute() error = %v, want coded error", err)
	}
	if coded.Code != protocol.CodeEINVAL {
		t.Fatalf("Code = %q, want %q", coded.Code, protocol.CodeEINVAL)
	}
	if coded.Message != "token authentication is not configured" {
		t.Fatalf("Message = %q, want %q", coded.Message, "token authentication is not configured")
	}
	if len(service.requests) != 0 {
		t.Fatalf("service requests = %#v, want none", service.requests)
	}
}

func TestDaemonExecutorWithTokenMapRequiresVersion2(t *testing.T) {
	service := &stubRequestService{}
	worker := &stubWorkerInvoker{}
	executor := daemonExecutor{
		service:  service,
		tokenMap: auth.NewTokenMap(map[string]auth.Identity{"token-a": {UID: 1001, GID: 1001}}),
		worker:   worker,
	}

	err := executor.Execute(context.Background(), protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
	})
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	coded, ok := protocol.AsCoded(err)
	if !ok {
		t.Fatalf("Execute() error = %v, want coded error", err)
	}
	if coded.Code != protocol.CodeEINVAL {
		t.Fatalf("Code = %q, want %q", coded.Code, protocol.CodeEINVAL)
	}
	if coded.Message != "token-authenticated requests require protocol version 2" {
		t.Fatalf("Message = %q, want %q", coded.Message, "token-authenticated requests require protocol version 2")
	}
	if len(worker.requests) != 0 {
		t.Fatalf("worker requests = %#v, want none", worker.requests)
	}
}

func TestDaemonExecutorWithTokenMapRejectsUnknownToken(t *testing.T) {
	service := &stubRequestService{}
	worker := &stubWorkerInvoker{}
	executor := daemonExecutor{
		service:  service,
		tokenMap: auth.NewTokenMap(map[string]auth.Identity{"token-a": {UID: 1001, GID: 1001}}),
		worker:   worker,
	}

	err := executor.Execute(context.Background(), protocol.Request{
		Version: protocol.Version2,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
		Token:   "token-b",
	})
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	coded, ok := protocol.AsCoded(err)
	if !ok {
		t.Fatalf("Execute() error = %v, want coded error", err)
	}
	if coded.Code != protocol.CodeEPERM {
		t.Fatalf("Code = %q, want %q", coded.Code, protocol.CodeEPERM)
	}
	if coded.Message != "invalid authentication token" {
		t.Fatalf("Message = %q, want %q", coded.Message, "invalid authentication token")
	}
	if len(worker.requests) != 0 {
		t.Fatalf("worker requests = %#v, want none", worker.requests)
	}
}

func TestDaemonExecutorWithTokenMapUsesMappedIdentity(t *testing.T) {
	service := &stubRequestService{}
	worker := &stubWorkerInvoker{}
	identity := auth.Identity{UID: 2001, GID: 2002, Groups: []uint32{44, 2003}}
	executor := daemonExecutor{
		service:  service,
		tokenMap: auth.NewTokenMap(map[string]auth.Identity{"token-a": identity}),
		worker:   worker,
	}

	req := protocol.Request{
		Version: protocol.Version2,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
		Token:   "token-a",
	}

	if err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(service.requests) != 0 {
		t.Fatalf("service requests = %#v, want none", service.requests)
	}
	if len(worker.requests) != 1 || worker.requests[0] != req {
		t.Fatalf("worker requests = %#v, want %#v", worker.requests, []protocol.Request{req})
	}
	if len(worker.identities) != 1 {
		t.Fatalf("worker identities = %#v, want one identity", worker.identities)
	}
	gotIdentity := worker.identities[0]
	if gotIdentity.Name != identity.Name || gotIdentity.UID != identity.UID || gotIdentity.GID != identity.GID {
		t.Fatalf("worker identity = %#v, want %#v", gotIdentity, identity)
	}
	if len(gotIdentity.Groups) != len(identity.Groups) {
		t.Fatalf("worker groups = %#v, want %#v", gotIdentity.Groups, identity.Groups)
	}
	for index, group := range identity.Groups {
		if gotIdentity.Groups[index] != group {
			t.Fatalf("worker groups = %#v, want %#v", gotIdentity.Groups, identity.Groups)
		}
	}
}

func TestCredentialForIdentity(t *testing.T) {
	current := auth.Identity{UID: 1000, GID: 1000, Groups: []uint32{44, 1002}}

	if credential := credentialForIdentity(auth.Identity{UID: 1000, GID: 1000, Groups: []uint32{1002, 44}}, current); credential != nil {
		t.Fatalf("credentialForIdentity() = %#v, want nil for current identity", credential)
	}

	credential := credentialForIdentity(auth.Identity{UID: 2000, GID: 2001, Groups: []uint32{3000}}, current)
	if credential == nil {
		t.Fatal("credentialForIdentity() unexpectedly returned nil")
	}
	if credential.Uid != 2000 || credential.Gid != 2001 {
		t.Fatalf("credential = %#v, want uid=2000 gid=2001", credential)
	}
	if len(credential.Groups) != 1 || credential.Groups[0] != 3000 {
		t.Fatalf("credential.Groups = %#v, want %#v", credential.Groups, []uint32{3000})
	}
}

func TestCurrentProcessIdentityExcludesPrimaryGroup(t *testing.T) {
	identity, err := currentProcessIdentity()
	if err != nil {
		t.Fatalf("currentProcessIdentity() error = %v", err)
	}

	for _, group := range identity.Groups {
		if group == identity.GID {
			t.Fatalf("currentProcessIdentity() groups = %#v, should not include primary gid %d", identity.Groups, identity.GID)
		}
	}
}

func TestRunWorkerSuccess(t *testing.T) {
	originalFactory := newWorkerService
	t.Cleanup(func() {
		newWorkerService = originalFactory
	})

	service := &stubRequestService{}
	newWorkerService = func(string) requestService { return service }

	req := protocol.Request{
		Version: protocol.Version2,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
		Token:   "token-a",
	}

	var input bytes.Buffer
	if err := framing.Write(&input, req); err != nil {
		t.Fatalf("framing.Write() error = %v", err)
	}

	var output bytes.Buffer
	if err := runWorker("/share", &input, &output); err != nil {
		t.Fatalf("runWorker() error = %v", err)
	}

	var resp protocol.Response
	if err := framing.Read(&output, &resp); err != nil {
		t.Fatalf("framing.Read() error = %v", err)
	}
	if !resp.OK {
		t.Fatalf("resp = %#v, want success", resp)
	}
	if len(service.requests) != 1 || service.requests[0] != req {
		t.Fatalf("service requests = %#v, want %#v", service.requests, []protocol.Request{req})
	}
}

func TestRunWorkerInvalidRequest(t *testing.T) {
	var output bytes.Buffer
	if err := runWorker("/share", bytes.NewReader(nil), &output); err != nil {
		t.Fatalf("runWorker() error = %v", err)
	}

	var resp protocol.Response
	if err := framing.Read(&output, &resp); err != nil {
		t.Fatalf("framing.Read() error = %v", err)
	}
	if resp.OK {
		t.Fatalf("resp = %#v, want failure", resp)
	}
	if resp.Error == nil || resp.Error.Code != protocol.CodeEINVAL {
		t.Fatalf("resp.Error = %#v, want EINVAL", resp.Error)
	}
}

func TestRunWorkerReturnsServiceFailure(t *testing.T) {
	originalFactory := newWorkerService
	t.Cleanup(func() {
		newWorkerService = originalFactory
	})

	service := &stubRequestService{err: errors.New("boom")}
	newWorkerService = func(string) requestService { return service }

	req := protocol.Request{
		Version: protocol.Version2,
		Op:      protocol.OpReflink,
		Src:     "src",
		Dst:     "dst",
		Token:   "token-a",
	}

	var input bytes.Buffer
	if err := framing.Write(&input, req); err != nil {
		t.Fatalf("framing.Write() error = %v", err)
	}

	var output bytes.Buffer
	if err := runWorker("/share", &input, &output); err != nil {
		t.Fatalf("runWorker() error = %v", err)
	}

	var resp protocol.Response
	if err := framing.Read(&output, &resp); err != nil {
		t.Fatalf("framing.Read() error = %v", err)
	}
	if resp.OK {
		t.Fatalf("resp = %#v, want failure", resp)
	}
	if resp.Error == nil || resp.Error.Code != protocol.CodeEINVAL || resp.Error.Message != "boom" {
		t.Fatalf("resp.Error = %#v, want EINVAL/boom", resp.Error)
	}
}
