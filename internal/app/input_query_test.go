package app

import (
	"strings"
	"testing"
)

func TestBuildInputQuery(t *testing.T) {
	tests := []struct {
		name    string
		items   []MetafieldsQueryItem
		want    string
		wantErr string // substring expected in the error; "" means no error
	}{
		{
			name:  "no entries returns empty no error",
			items: nil,
			want:  "",
		},
		{
			name:  "single product entry",
			items: []MetafieldsQueryItem{{Resource: "product", Namespace: "custom-option", Key: "adjust-price"}},
			want:  `{"metafields_query":[{"resource":"product","namespace":"custom-option","key":"adjust-price"}]}`,
		},
		{
			name:  "single shop entry",
			items: []MetafieldsQueryItem{{Resource: "shop", Namespace: "custom-option", Key: "global-config"}},
			want:  `{"metafields_query":[{"resource":"shop","namespace":"custom-option","key":"global-config"}]}`,
		},
		{
			name: "multiple entries preserve order",
			items: []MetafieldsQueryItem{
				{Resource: "product", Namespace: "ns", Key: "a"},
				{Resource: "shop", Namespace: "ns", Key: "b"},
			},
			want: `{"metafields_query":[{"resource":"product","namespace":"ns","key":"a"},{"resource":"shop","namespace":"ns","key":"b"}]}`,
		},
		{
			name:    "invalid resource errors",
			items:   []MetafieldsQueryItem{{Resource: "variant", Namespace: "ns", Key: "a"}},
			wantErr: "resource",
		},
		{
			name:    "empty namespace errors",
			items:   []MetafieldsQueryItem{{Resource: "product", Namespace: "", Key: "a"}},
			wantErr: "namespace",
		},
		{
			name:    "empty key errors",
			items:   []MetafieldsQueryItem{{Resource: "product", Namespace: "ns", Key: ""}},
			wantErr: "key",
		},
		{
			name: "over limit errors",
			items: []MetafieldsQueryItem{
				{Resource: "product", Namespace: "ns", Key: "1"},
				{Resource: "product", Namespace: "ns", Key: "2"},
				{Resource: "product", Namespace: "ns", Key: "3"},
				{Resource: "product", Namespace: "ns", Key: "4"},
				{Resource: "product", Namespace: "ns", Key: "5"},
				{Resource: "product", Namespace: "ns", Key: "6"},
			},
			wantErr: "max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildInputQuery(tt.items)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (out=%q)", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  = %q\nwant = %q", got, tt.want)
			}
		})
	}
}
