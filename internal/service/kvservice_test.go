package service

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"testing"

	"github.com/shauntso/hoodb/internal/storage"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "[test] ", log.LstdFlags)
}

type testSnapshotSink struct {
	buf      bytes.Buffer
	canceled bool
}

func (s *testSnapshotSink) Write(p []byte) (n int, err error) { return s.buf.Write(p) }
func (s *testSnapshotSink) Close() error                      { return nil }
func (s *testSnapshotSink) ID() string                        { return "test-snap-1" }
func (s *testSnapshotSink) Cancel() error                     { s.canceled = true; return nil }

func TestFSMSnapshotPersist_Binary(t *testing.T) {
	tmpDir := "./test_db_snapshot"
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	testData := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "a longer value with special chars",
	}
	for k, v := range testData {
		if err := store.Put([]byte(k), []byte(v)); err != nil {
			t.Fatalf("Failed to put: %v", err)
		}
	}

	snapshot := store.GetSnapshot()
	defer store.ReleaseSnapshot(snapshot)

	fsnap := &fsmSnapshot{
		store:    store,
		snapshot: snapshot,
	}

	sink := &testSnapshotSink{}
	if err := fsnap.Persist(sink); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	if sink.canceled {
		t.Fatal("Snapshot was canceled unexpectedly")
	}

	data := sink.buf.Bytes()
	if !bytes.HasPrefix(data, snapshotMagic) {
		t.Fatalf("Missing magic header")
	}

	reader := bytes.NewReader(data[len(snapshotMagic):])
	decoded := make(map[string]string)
	lbuf := make([]byte, 4)

	for {
		if _, rerr := io.ReadFull(reader, lbuf); rerr != nil {
			if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
				break
			}
			t.Fatalf("Read key length error: %v", rerr)
		}
		keyLen := binary.BigEndian.Uint32(lbuf)
		key := make([]byte, keyLen)
		if _, rerr := io.ReadFull(reader, key); rerr != nil {
			t.Fatalf("Read key error: %v", rerr)
		}
		if _, rerr := io.ReadFull(reader, lbuf); rerr != nil {
			t.Fatalf("Read value length error: %v", rerr)
		}
		valueLen := binary.BigEndian.Uint32(lbuf)
		value := make([]byte, valueLen)
		if _, rerr := io.ReadFull(reader, value); rerr != nil {
			t.Fatalf("Read value error: %v", rerr)
		}
		decoded[string(key)] = string(value)
	}

	if len(decoded) != len(testData) {
		t.Errorf("expected %d entries, decoded %d", len(testData), len(decoded))
	}
	for k, v := range testData {
		if decoded[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, decoded[k])
		}
	}
}

func TestFSMRestore_BinaryFormat(t *testing.T) {
	tmpDir := "./test_db_restore_binary"
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	var snapBuf bytes.Buffer
	snapBuf.Write(snapshotMagic)

	testData := map[string]string{
		"restored_key1": "restored_value1",
		"restored_key2": "restored_value2",
		"restored_key3": "restored_value3",
	}

	lenBuf := make([]byte, 4)
	for k, v := range testData {
		binary.BigEndian.PutUint32(lenBuf, uint32(len(k)))
		snapBuf.Write(lenBuf)
		snapBuf.WriteString(k)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(v)))
		snapBuf.Write(lenBuf)
		snapBuf.WriteString(v)
	}

	kv := &KVService{
		store:      store,
		raftToHTTP: make(map[string]string),
	}
	kv.logger = testLogger()

	rc := io.NopCloser(&snapBuf)
	if err := kv.Restore(rc); err != nil {
		t.Fatalf("Restore binary failed: %v", err)
	}

	for k, expectedV := range testData {
		val, gerr := store.Get([]byte(k))
		if gerr != nil {
			t.Errorf("Get %q error: %v", k, gerr)
			continue
		}
		if string(val) != expectedV {
			t.Errorf("key %q: expected %q, got %q", k, expectedV, string(val))
		}
	}
}

func TestFSMRestore_JSONLegacy(t *testing.T) {
	tmpDir := "./test_db_restore_json"
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	testData := map[string]string{
		"json_key1": "json_value1",
		"json_key2": "json_value2",
	}
	jsonData, _ := json.Marshal(testData)

	kv := &KVService{
		store:      store,
		raftToHTTP: make(map[string]string),
	}
	kv.logger = testLogger()

	rc := io.NopCloser(bytes.NewReader(jsonData))
	if err := kv.Restore(rc); err != nil {
		t.Fatalf("Restore JSON failed: %v", err)
	}

	for k, expectedV := range testData {
		val, gerr := store.Get([]byte(k))
		if gerr != nil {
			t.Errorf("Get %q error: %v", k, gerr)
			continue
		}
		if string(val) != expectedV {
			t.Errorf("key %q: expected %q, got %q", k, expectedV, string(val))
		}
	}
}

func TestFSMSnapshotRoundTrip(t *testing.T) {
	tmpDirSrc := "./test_db_roundtrip_src"
	tmpDirDst := "./test_db_roundtrip_dst"
	defer os.RemoveAll(tmpDirSrc)
	defer os.RemoveAll(tmpDirDst)

	srcStore, err := storage.NewPebbleStore(tmpDirSrc)
	if err != nil {
		t.Fatalf("Failed to create src store: %v", err)
	}
	defer srcStore.Close()

	testData := map[string]string{}
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("rkey_%04d", i)
		v := fmt.Sprintf("rval_%04d", i)
		testData[k] = v
		if err := srcStore.Put([]byte(k), []byte(v)); err != nil {
			t.Fatalf("Failed to put: %v", err)
		}
	}

	snapshot := srcStore.GetSnapshot()
	fsnap := &fsmSnapshot{store: srcStore, snapshot: snapshot}

	sink := &testSnapshotSink{}
	if err := fsnap.Persist(sink); err != nil {
		srcStore.ReleaseSnapshot(snapshot)
		t.Fatalf("Persist failed: %v", err)
	}
	fsnap.Release()

	dstStore, err := storage.NewPebbleStore(tmpDirDst)
	if err != nil {
		t.Fatalf("Failed to create dst store: %v", err)
	}
	defer dstStore.Close()

	kvDst := &KVService{
		store:      dstStore,
		raftToHTTP: make(map[string]string),
	}
	kvDst.logger = testLogger()

	rc := io.NopCloser(bytes.NewReader(sink.buf.Bytes()))
	if err := kvDst.Restore(rc); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	for k, expectedV := range testData {
		val, gerr := dstStore.Get([]byte(k))
		if gerr != nil {
			t.Errorf("Get %q error: %v", k, gerr)
			continue
		}
		if string(val) != expectedV {
			t.Errorf("key %q: expected %q, got %q", k, expectedV, string(val))
		}
	}
}
