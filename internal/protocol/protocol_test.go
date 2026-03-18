package protocol

import (
	"errors"
	"io/fs"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  Request
		ok   bool
	}{
		{
			name: "valid",
			req: Request{
				Version: Version1,
				Op:      OpReflink,
				Src:     "src",
				Dst:     "dst",
			},
			ok: true,
		},
		{
			name: "valid v2",
			req: Request{
				Version: Version2,
				Op:      OpReflink,
				Src:     "src",
				Dst:     "dst",
				Token:   "secret",
			},
			ok: true,
		},
		{
			name: "bad version",
			req: Request{
				Version: 99,
				Op:      OpReflink,
				Src:     "src",
				Dst:     "dst",
			},
		},
		{
			name: "bad op",
			req: Request{
				Version: Version1,
				Op:      "copy",
				Src:     "src",
				Dst:     "dst",
			},
		},
		{
			name: "same path",
			req: Request{
				Version: Version1,
				Op:      OpReflink,
				Src:     "same",
				Dst:     "same",
			},
		},
		{
			name: "v2 missing token",
			req: Request{
				Version: Version2,
				Op:      OpReflink,
				Src:     "src",
				Dst:     "dst",
			},
		},
		{
			name: "v1 token rejected",
			req: Request{
				Version: Version1,
				Op:      OpReflink,
				Src:     "src",
				Dst:     "dst",
				Token:   "secret",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.req.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("Validate() unexpectedly succeeded")
			}
		})
	}
}

func TestMessageFromError(t *testing.T) {
	t.Parallel()

	t.Run("raw permission denied", func(t *testing.T) {
		t.Parallel()

		if got := MessageFromError(fs.ErrPermission); got != "permission denied" {
			t.Fatalf("MessageFromError() = %q, want %q", got, "permission denied")
		}
	})

	t.Run("coded path containment preserved", func(t *testing.T) {
		t.Parallel()

		err := WrapError(CodeEPERM, "path must stay within the shared root", errors.New("boom"))
		if got := MessageFromError(err); got != "path must stay within the shared root" {
			t.Fatalf("MessageFromError() = %q, want %q", got, "path must stay within the shared root")
		}
	})
}
