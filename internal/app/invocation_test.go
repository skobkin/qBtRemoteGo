package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseInvocationArgs(t *testing.T) {
	torrentPath := filepath.Join(t.TempDir(), "example.torrent")
	if err := os.WriteFile(torrentPath, []byte("torrent"), 0o600); err != nil {
		t.Fatalf("write torrent fixture: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want InvocationBatch
	}{
		{
			name: "collects multiple magnets",
			args: []string{"magnet:?xt=urn:btih:abc", " magnet:?xt=urn:btih:def "},
			want: InvocationBatch{
				MagnetLinks: []string{"magnet:?xt=urn:btih:abc", "magnet:?xt=urn:btih:def"},
			},
		},
		{
			name: "collects existing torrent files",
			args: []string{torrentPath},
			want: InvocationBatch{
				TorrentFiles: []string{torrentPath},
			},
		},
		{
			name: "collects mixed inputs and skips invalid torrent paths",
			args: []string{"magnet:?xt=urn:btih:abc", filepath.Join(t.TempDir(), "missing.torrent"), torrentPath},
			want: InvocationBatch{
				MagnetLinks:  []string{"magnet:?xt=urn:btih:abc"},
				TorrentFiles: []string{torrentPath},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseInvocationArgs(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseInvocationArgs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
