//go:build windows

package service

import (
	"testing"

	kservice "github.com/kardianos/service"
)

func TestParseWindowsServiceStatusUsesNumericStateCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		output string
		want   kservice.Status
	}{
		{
			name: "running with localized label",
			output: "SERVICE_NAME: ClashForClaw\r\n" +
				"        STATE              : 4  正在运行\r\n",
			want: kservice.StatusRunning,
		},
		{
			name: "stopped with localized label",
			output: "SERVICE_NAME: ClashForClaw\r\n" +
				"        STATE              : 1  已停止\r\n",
			want: kservice.StatusStopped,
		},
		{
			name: "stop pending remains unknown",
			output: "SERVICE_NAME: ClashForClaw\r\n" +
				"        STATE              : 3  正在停止\r\n",
			want: kservice.StatusUnknown,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parseWindowsServiceStatus(tc.output); got != tc.want {
				t.Fatalf("parseWindowsServiceStatus() = %v, want %v", got, tc.want)
			}
		})
	}
}
