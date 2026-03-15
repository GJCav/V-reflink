package protocol

import "testing"

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
