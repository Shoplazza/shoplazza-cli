package dynamic

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

func TestIsDestructive(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   []string
		want   bool
	}{
		{"http delete", "DELETE", []string{"products", "delete"}, true},
		{"http delete lowercase", "delete", []string{"webhooks", "remove"}, true},
		{"cancel verb over post", "POST", []string{"orders", "cancel"}, true},
		{"batch-delete verb", "POST", []string{"products", "batch-delete"}, true},
		{"unpublish verb", "PUT", []string{"products", "unpublish"}, true},
		{"undeploy verb", "POST", []string{"checkout", "undeploy"}, true},
		{"get is safe", "GET", []string{"orders", "get"}, false},
		{"create is safe", "POST", []string{"products", "create"}, false},
		{"update is safe", "PUT", []string{"products", "update"}, false},
		{"list is safe", "GET", []string{"orders", "list"}, false},
		{"empty path", "GET", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := registry.Command{HTTP: registry.HTTP{Method: tt.method}, Path: tt.path}
			if got := isDestructive(c); got != tt.want {
				t.Errorf("isDestructive(method=%s path=%v) = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}
