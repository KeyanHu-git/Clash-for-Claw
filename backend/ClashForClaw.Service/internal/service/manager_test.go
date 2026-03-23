package service

import (
	"errors"
	"testing"
)

func TestHasAccessDeniedMatchesLocalizedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "english",
			err:  errors.New("Access is denied."),
			want: true,
		},
		{
			name: "chinese",
			err:  errors.New("错误: 拒绝访问。"),
			want: true,
		},
		{
			name: "other",
			err:  errors.New("service_not_installed"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasAccessDenied(tc.err); got != tc.want {
				t.Fatalf("hasAccessDenied(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
