package main

import "testing"

func TestInitOptsValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    initOpts
		wantErr bool
	}{
		{"interactive default ok", initOpts{}, false},
		{"yes without profile fails", initOpts{yes: true}, true},
		{"no-tui without profile fails", initOpts{noTUI: true}, true},
		{"yes with valid profile ok", initOpts{yes: true, profile: "recommended"}, false},
		{"no-tui with valid profile ok", initOpts{noTUI: true, profile: "minimal"}, false},
		{"unknown profile fails", initOpts{profile: "kitchen-sink"}, true},
		{"profile=full ok", initOpts{yes: true, profile: "full"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
