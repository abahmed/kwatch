package app

import "testing"

func TestParseVolumeLimit(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int64
		wantErr bool
	}{
		{name: "unset means no limit", raw: "", want: 0},
		{name: "blank means no limit", raw: "  ", want: 0},
		{name: "binary suffix", raw: "2Gi", want: 2 << 30},
		{name: "plain bytes", raw: "4096", want: 4096},
		{name: "garbage is rejected", raw: "lots", wantErr: true},
		{name: "negative is rejected", raw: "-1Gi", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVolumeLimit(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("limit = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestVolumeLimitReadsEnvironment(t *testing.T) {
	t.Setenv(volumeLimitEnv, "1Mi")
	got, err := volumeLimit()
	if err != nil || got != 1<<20 {
		t.Fatalf("volumeLimit() = %d, %v", got, err)
	}
	t.Setenv(volumeLimitEnv, "")
	if got, err = volumeLimit(); err != nil || got != 0 {
		t.Fatalf("unset volumeLimit() = %d, %v", got, err)
	}
}
