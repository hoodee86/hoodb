package storage

import (
	"os"
	"testing"
)

func TestScanPrefix(t *testing.T) {
	tmpDir := "./test_db_scan"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 写入测试数据
	testData := map[string]string{
		"user:1":      "alice",
		"user:2":      "bob",
		"user:3":      "charlie",
		"user:10":     "dave",
		"product:1":   "laptop",
		"product:2":   "phone",
		"order:100":   "order1",
		"bench_12345": "benchval",
	}

	for k, v := range testData {
		if err := store.Put([]byte(k), []byte(v)); err != nil {
			t.Fatalf("Failed to put %s: %v", k, err)
		}
	}

	// 测试前缀扫描: "user:"
	results, nextCursor, err := store.ScanPrefix([]byte("user:"), 100, nil)
	if err != nil {
		t.Fatalf("ScanPrefix failed: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 user: keys, got %d", len(results))
	}
	if nextCursor != "" {
		t.Errorf("expected empty cursor (no more), got %q", nextCursor)
	}

	// 测试前缀扫描: "product:"
	results, _, err = store.ScanPrefix([]byte("product:"), 100, nil)
	if err != nil {
		t.Fatalf("ScanPrefix failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 product: keys, got %d", len(results))
	}

	// 测试分页: limit=2
	results, nextCursor, err = store.ScanPrefix([]byte("user:"), 2, nil)
	if err != nil {
		t.Fatalf("ScanPrefix failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit 2, got %d", len(results))
	}
	if nextCursor == "" {
		t.Error("expected non-empty cursor for next page")
	}

	// 使用游标获取下一页
	results2, nextCursor2, err := store.ScanPrefix([]byte("user:"), 2, []byte(nextCursor))
	if err != nil {
		t.Fatalf("ScanPrefix with cursor failed: %v", err)
	}
	if len(results2) != 2 {
		t.Errorf("expected 2 results in page 2, got %d", len(results2))
	}
	if nextCursor2 != "" {
		t.Errorf("expected empty cursor after last page, got %q", nextCursor2)
	}

	// 确保两页不重叠
	page1Keys := make(map[string]bool)
	for _, r := range results {
		page1Keys[r.Key] = true
	}
	for _, r := range results2 {
		if page1Keys[r.Key] {
			t.Errorf("key %q appears in both pages", r.Key)
		}
	}

	// 测试不存在的前缀
	results, _, err = store.ScanPrefix([]byte("nonexistent:"), 100, nil)
	if err != nil {
		t.Fatalf("ScanPrefix for nonexistent prefix failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent prefix, got %d", len(results))
	}

	// 测试空前缀 (扫描所有)
	results, _, err = store.ScanPrefix(nil, 100, nil)
	if err != nil {
		t.Fatalf("ScanPrefix with nil prefix failed: %v", err)
	}
	if len(results) != len(testData) {
		t.Errorf("expected %d results for empty prefix, got %d", len(testData), len(results))
	}
}

func TestPrefixUpperBound(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"simple", []byte("abc"), []byte("abd")},
		{"single byte", []byte("a"), []byte("b")},
		{"trailing 0xff", []byte{0x61, 0xff}, []byte{0x62}},
		{"all 0xff", []byte{0xff, 0xff}, nil},
		{"empty", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prefixUpperBound(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if string(result) != string(tt.expected) {
					t.Errorf("expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}
